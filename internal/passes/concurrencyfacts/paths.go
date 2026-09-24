package concurrencyfacts

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Bounded paths preserve differing effects rather than merging them into one
// unconditional history. Correlations are deliberately over-approximated:
// consumers must prove a dependency on every path, including possible escapes.
// Loops, opaque instructions, and excessive products discard the whole answer.
const maxProtocolPaths = 8

func (engine *Engine) collectPaths(function *ssa.Function, root bool) Summary {
	if !detachedRecovery(function) {
		engine.recordBlockCutoff(function.Recover, cutoffRecovery)
		return Summary{Reason: ReasonControlFlowUnknown}
	}
	flow, reason := engine.orderedBlocks(function, root)
	if reason != ReasonNone {
		return Summary{Reason: reason}
	}
	states := map[*ssa.BasicBlock][]Summary{function.Blocks[0]: {{}}}
	var paths []Summary
	for _, block := range flow.order {
		// A panicking block contributes no alternative: it never returns
		// normally, so none of its states can reach a later event.
		if panics(block) {
			continue
		}
		folded := flow.isFolded(block)
		current, reason := engine.advanceBlock(states[block], flow, block, root)
		if reason != ReasonNone {
			return Summary{Reason: reason}
		}
		successors := flow.successors(block)
		if len(successors) == 0 {
			returned := returnedFacts(block)
			for index := range current {
				current[index].Returned = returned
			}
			paths = append(paths, current...)
			if len(paths) > maxProtocolPaths {
				engine.recordBlockCutoff(block, cutoffBranch)
				return Summary{Reason: ReasonAlternativeLimit}
			}
		}
		for _, next := range successors {
			// A loop's exit is not one branch choice, so it adds no condition.
			condition, branching := branchCondition(block, next)
			branching = branching && !folded
			for _, state := range current {
				state = cloneEffects(state)
				if branching {
					state.Conditions = append(state.Conditions, condition)
				}
				states[next] = append(states[next], state)
			}
			if len(states[next]) > maxProtocolPaths {
				engine.recordBlockCutoff(block, cutoffBranch)
				return Summary{Reason: ReasonAlternativeLimit}
			}
		}
	}
	return finishPaths(paths)
}

// Pending defers or an incomplete worker cannot become a completed path just
// because control flow reached a terminal block. Cancellation requirements
// remain attached separately to each alternative until call-site binding.
func finishPaths(paths []Summary) Summary {
	if len(paths) == 0 {
		return Summary{Reason: ReasonControlFlowUnknown}
	}
	for index := range paths {
		if len(paths[index].deferred) != 0 {
			return Summary{Reason: ReasonDeferredEffectsUnknown}
		}
		// An alternative is a complete sequence, never one with a hole.
		if !paths[index].CallbacksBound() {
			return Summary{Reason: ReasonCallbackUnknown}
		}
		if paths[index].hasWorkerAlternatives() {
			paths[index].Reason = ReasonSelectAlternatives
			paths[index].AlternativesComplete = true
		}
		paths[index] = finishCancellation(paths[index])
	}
	return Summary{Paths: paths, Reason: ReasonBranchAlternatives}
}

// advanceBlock adds one block's effects to every state: a folded loop's
// replay (see loops.go), or the block's own instructions.
func (engine *Engine) advanceBlock(states []Summary, flow acyclicFlow, block *ssa.BasicBlock, root bool) ([]Summary, Reason) {
	if folded, ok := flow.folded[block]; ok {
		for index := range states {
			if reason := engine.replayLoop(&states[index], folded, root); reason != ReasonNone {
				return nil, reason
			}
		}
		return states, ReasonNone
	}
	for _, instruction := range block.Instrs {
		var reason Reason
		states, reason = engine.advancePaths(states, instruction, root)
		if reason != ReasonNone {
			engine.recordCutoff(instruction, cutoffInstruction)
			return nil, reason
		}
	}
	return states, ReasonNone
}

func (engine *Engine) advancePaths(states []Summary, instruction ssa.Instruction, root bool) ([]Summary, Reason) {
	var next []Summary
	for _, state := range states {
		if !engine.budget.Spend() {
			return nil, ReasonBudgetExhausted
		}
		branches, reason, handled := engine.appendPathCall(state, instruction)
		if handled {
			if reason != ReasonNone {
				return nil, reason
			}
			next = append(next, branches...)
		} else {
			if reason := engine.appendInstruction(&state, instruction, root); reason != ReasonNone {
				return nil, reason
			}
			next = append(next, state)
		}
		if len(next) > maxProtocolPaths {
			return nil, ReasonAlternativeLimit
		}
	}
	for _, state := range next {
		if state.operationCount() > maxOperations {
			return nil, ReasonSummaryLimit
		}
	}
	return next, ReasonNone
}

// Splice a helper's complete alternatives into independent copies of the
// caller prefix. Never combine effects taken from different helper returns.
func (engine *Engine) appendPathCall(state Summary, instruction ssa.Instruction) ([]Summary, Reason, bool) {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return nil, ReasonNone, false
	}
	called := engine.callSummary(call)
	if len(called.Paths) == 0 {
		return nil, ReasonNone, false
	}
	var branches []Summary
	for _, path := range called.Paths {
		branch := cloneEffects(state)
		if reason := appendCalled(&branch, path, call); reason != ReasonNone {
			return nil, reason, true
		}
		branch.Conditions = append(branch.Conditions, resultConditions(call, path.Returned)...)
		branches = append(branches, branch)
	}
	return branches, ReasonNone, true
}

// Every alternative must bind to this invocation's values. One unavailable
// binding invalidates the set; dropping it would remove a possible escape.
func (engine *Engine) bindPaths(paths []Summary, bindings []ssaflow.CallBinding, instruction ssa.CallInstruction) Summary {
	var result []Summary
	for _, path := range paths {
		bound := engine.bindSummary(path, bindings, instruction)
		if !composableLinear(bound) && !bound.AlternativesComplete {
			return Summary{Reason: bound.Reason}
		}
		result = append(result, bound)
	}
	return Summary{Paths: result, Reason: ReasonBranchAlternatives}
}

// branchCondition returns the choice a block's If makes to reach next.
func branchCondition(block, next *ssa.BasicBlock) (Condition, bool) {
	if len(block.Instrs) == 0 || len(block.Succs) != 2 || block.Succs[0] == block.Succs[1] {
		return Condition{}, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return Condition{}, false
	}
	taken := next == block.Succs[0]
	if comparison, ok := branch.Cond.(*ssa.BinOp); ok && (comparison.Op == token.EQL || comparison.Op == token.NEQ) {
		if subject, compared, ok := constantOperand(comparison); ok {
			return Condition{Value: subject, Compared: compared, Holds: taken == (comparison.Op == token.EQL)}, true
		}
	}
	return Condition{Value: branch.Cond, Holds: taken}, true
}

// constantOperand splits a comparison with one constant side.
func constantOperand(comparison *ssa.BinOp) (ssa.Value, *ssa.Const, bool) {
	if compared, ok := comparison.Y.(*ssa.Const); ok {
		return comparison.X, compared, true
	}
	if compared, ok := comparison.X.(*ssa.Const); ok {
		return comparison.Y, compared, true
	}
	return nil, nil, false
}

// Context chains are bounded like cutoff chains; a deeper binding keeps the
// innermost sites, which is enough to keep separate calls apart.
const maxConditionContext = 8

// boundConditions maps a callee's conditions into the caller. A condition that
// tests one of the callee's own parameters directly, possibly negated, becomes
// a condition on the caller's argument, so it relates exactly to the caller's
// own tests of that value, and a constant argument folds it. Every other
// condition stays a callee condition, tagged with this call site so separate
// calls stay apart.
func boundConditions(conditions []Condition, bindings []ssaflow.CallBinding, site token.Pos) []Condition {
	if len(conditions) == 0 {
		return nil
	}
	bound := make([]Condition, 0, len(conditions))
	for _, condition := range conditions {
		if argument, holds, ok := parameterCondition(condition, bindings); ok {
			bound = append(bound, Condition{Value: argument, Compared: condition.Compared, Holds: holds})
			continue
		}
		if len(condition.Context) < maxConditionContext {
			condition.Context = append(slices.Clone(condition.Context), site)
		}
		bound = append(bound, condition)
	}
	return bound
}

// parameterCondition resolves a local condition on a callee parameter, or a
// comparison of one with a constant, to the caller's argument. A condition
// already bound through a callee has no parameter of this callee to resolve.
func parameterCondition(condition Condition, bindings []ssaflow.CallBinding) (ssa.Value, bool, bool) {
	if len(condition.Context) != 0 {
		return nil, false, false
	}
	value, holds := condition.Value, condition.Holds
	for condition.Compared == nil {
		negation, ok := value.(*ssa.UnOp)
		if !ok || negation.Op != token.NOT {
			break
		}
		value, holds = negation.X, !holds
	}
	parameter, ok := value.(*ssa.Parameter)
	if !ok {
		return nil, false, false
	}
	for _, binding := range bindings {
		if binding.Local == parameter && !binding.Captured {
			return binding.Supplied, holds, true
		}
	}
	return nil, false, false
}

// returnedFacts records what a terminal block returns where that is certain.
// A boxed value is a non-nil interface even when it boxes a nil pointer, and
// an allocation is never nil.
func returnedFacts(block *ssa.BasicBlock) []Returned {
	if len(block.Instrs) == 0 {
		return nil
	}
	returned, ok := block.Instrs[len(block.Instrs)-1].(*ssa.Return)
	if !ok {
		return nil
	}
	var facts []Returned
	for index, result := range returned.Results {
		switch result := result.(type) {
		case *ssa.Const:
			facts = append(facts, Returned{Index: index, Constant: result})
		case *ssa.MakeInterface, *ssa.Alloc:
			facts = append(facts, Returned{Index: index})
		}
	}
	return facts
}

// resultConditions turns a spliced path's return facts into conditions on
// the caller's own result values, so its tests of a result relate exactly to
// the path that produced it. A result the caller never extracts adds none.
func resultConditions(call *ssa.Call, returned []Returned) []Condition {
	results := call.Common().Signature().Results().Len()
	var conditions []Condition
	for _, fact := range returned {
		index := fact.Index
		if results == 1 {
			index = -1
		}
		value := ssaflow.CallResult(call, index)
		if value == nil {
			continue
		}
		if fact.Constant != nil {
			conditions = append(conditions, Condition{Value: value, Compared: fact.Constant, Holds: true, Implied: true})
			continue
		}
		conditions = append(conditions, Condition{Value: value, Compared: ssa.NewConst(nil, value.Type()), Holds: false, Implied: true})
	}
	return conditions
}
