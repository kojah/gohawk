package lifecycle

import (
	"go/constant"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Conditional completion preserves a narrow relation between a synchronous
// call's result and its cleanup effect. The condition belongs to that call's
// normal returns, never to every nested helper. Only explicit Boolean and
// error-nil tests are accepted; unresolved results and dispatch stay opaque.
// The condition is an ssaflow.CallCondition with a result test and no
// arguments; OutcomeAny is the ordinary, unconditional search.

// ProveCompletionOnEdge asks whether the call result tested by from establishes
// completion of request.Target on the edge to to. Instruction is derived from
// the test; only synchronous calls and exact target mappings are eligible.
// The result says nothing about the opposite edge or an untested call.
func ProveCompletionOnEdge(from, to *ssa.BasicBlock, request CompletionRequest) proofs.CompletionProof {
	call, condition, ok := completionEdgeCondition(from, to)
	if !ok {
		return proofs.CompletionProof{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}}
	}
	request.Instruction, request.condition = call, condition
	request.ExactTarget, request.Coverage = true, CoverageEveryReturn
	return ProveCompletion(request)
}

func completionEdgeCondition(from, to *ssa.BasicBlock) (*ssa.Call, ssacall.CallCondition, bool) {
	if from == nil || len(from.Instrs) == 0 || len(from.Succs) != 2 || from.Succs[0] == from.Succs[1] {
		return nil, ssacall.CallCondition{}, false
	}
	branch, ok := from.Instrs[len(from.Instrs)-1].(*ssa.If)
	if !ok || to != from.Succs[0] && to != from.Succs[1] {
		return nil, ssacall.CallCondition{}, false
	}
	value, outcome := completionTest(branch.Cond, to == from.Succs[0])
	call, index, ok := ssacall.CallResultSource(value)
	if !ok || outcome == ssacall.OutcomeAny || !cfg.InstructionDominates(call, branch) {
		return nil, ssacall.CallCondition{}, false
	}
	return call, ssacall.CallCondition{Result: index, Outcome: outcome}, true
}

func completionTest(value ssa.Value, truth bool) (ssa.Value, ssacall.Outcome) {
	value, negated := ssaflow.BooleanNegationSource(value)
	truth = truth != negated
	if comparison, ok := value.(*ssa.BinOp); ok {
		return completionComparison(comparison, truth)
	}
	if truth {
		return value, ssacall.OutcomeTrue
	}
	return value, ssacall.OutcomeFalse
}

func completionComparison(comparison *ssa.BinOp, truth bool) (ssa.Value, ssacall.Outcome) {
	if comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return nil, ssacall.OutcomeAny
	}
	operand, literal := comparison.X, comparison.Y
	if _, ok := literal.(*ssa.Const); !ok {
		operand, literal = literal, operand
	}
	c, ok := literal.(*ssa.Const)
	if !ok {
		return nil, ssacall.OutcomeAny
	}
	equal := truth == (comparison.Op == token.EQL)
	if c.IsNil() && syntax.IsErrorType(operand.Type()) {
		if equal {
			return operand, ssacall.OutcomeNil
		}
		return operand, ssacall.OutcomeNonNil
	}
	if c.Value == nil || c.Value.Kind() != constant.Bool {
		return nil, ssacall.OutcomeAny
	}
	return completionTest(operand, equal == constant.BoolVal(c.Value))
}

type conditionalCompletionState struct {
	block       *ssa.BasicBlock
	predecessor *ssa.BasicBlock
	completed   bool
}

func (search *completionSearch) conditionalCoverage(
	function *ssa.Function, locals []mappedLocal, target ssa.Value, condition ssacall.CallCondition,
) bool {
	matched, failed := false, false
	initial := []conditionalCompletionState{{block: function.Blocks[0]}}
	cfg.WalkStatesWithin(initial, func(state conditionalCompletionState) conditionalCompletionState { return state },
		func(state conditionalCompletionState) ([]conditionalCompletionState, bool) {
			for _, instruction := range state.block.Instrs {
				if !search.budget.Spend() {
					failed = true
					return nil, false
				}
				state.completed = state.completed || search.instructionCompletes(instruction, locals, target)
				if ssapath.InstructionTerminatesControlFlow(instruction) {
					return nil, true
				}
				if returned, ok := instruction.(*ssa.Return); ok {
					covered, relevant := search.conditionalReturn(returned, locals, condition, state.completed)
					matched = matched || relevant
					failed = failed || relevant && !covered
					return nil, !failed
				}
			}
			var next []conditionalCompletionState
			successors := ssapath.SuccessorPolicy{Constants: search.constants}.SuccessorsWithin(state.block, state.predecessor, search.budget)
			for _, successor := range successors {
				if !search.budget.Spend() {
					failed = true
					return nil, false
				}
				next = append(next, conditionalCompletionState{block: successor, predecessor: state.block, completed: state.completed})
			}
			return next, true
		}, search.budget)
	return matched && !failed && !search.budget.Exhausted()
}

func (search *completionSearch) conditionalReturn(
	returned *ssa.Return, locals []mappedLocal, condition ssacall.CallCondition, completed bool,
) (covered, relevant bool) {
	if condition.Result >= len(returned.Results) {
		return false, true
	}
	value := returned.Results[condition.Result]
	// Resolve compiler-spilled Boolean results, but do not peel an error's
	// interface box: a typed nil error is not the nil error result.
	boolean := condition.Outcome == ssacall.OutcomeTrue || condition.Outcome == ssacall.OutcomeFalse
	if load, ok := value.(*ssa.UnOp); ok && load.Op == token.MUL && boolean {
		if resolved := heapmodel.NewStorage(search.budget).Resolve(value); resolved.Proven() {
			value = resolved.Value
		}
	}
	if holds, known := ssacall.OutcomeOf(condition.Outcome, value); known && !holds {
		return true, false
	}
	if completed {
		return true, true
	}
	call, index, ok := ssacall.CallResultSource(value)
	if !ok {
		return false, true
	}
	// A forwarding return composes the same result predicate with the nested
	// callee. Ordinary calls above were searched without this predicate.
	nested := *search
	nested.condition = ssacall.CallCondition{Result: index, Outcome: condition.Outcome}
	for _, local := range locals {
		if local.kind == localExact {
			if nested.completes(call, local.local).proven {
				return true, true
			}
		}
	}
	return false, true
}

// Result-guarded deferred completion. A deferred literal runs after the
// return statement has set the function's named results, so a literal that
// completes the target only while a named result holds some outcome, as the
// close-on-error idiom closes only while err is non-nil, completes on some
// returns and not on others. Asking once, at the defer, can only say "may"
// or "never". These queries ask per return instead: which deferred literals
// turn on a named result, and whether one completes the target given the
// value a particular return stores. Whether an unknown value suppresses a
// diagnostic, and what else settles the obligation on that path, is the
// caller's policy.

// ResultGuard is a deferred literal whose completion of a target turns on
// named results of the function that defers it.
type ResultGuard struct {
	Defer *ssa.Defer
	// Cells are the named-result cells the literal captures.
	Cells []*ssa.Alloc
}

// ResultGuardsProof publishes only a complete census of modeled result guards.
// Proven means discovery completed, including when Guards is empty; it does
// not assert completion at any return of the enclosing function.
type ResultGuardsProof struct {
	proofs.Proof
	Guards []ResultGuard
}

// ProveResultGuards shares request.Budget across instruction, capture,
// named-result and opposing completion questions. Cutoff discards all guards;
// completed opaque completion answers retain the ordinary discovery policy.
func ProveResultGuards(function *ssa.Function, request CompletionRequest) ResultGuardsProof {
	unknown := ResultGuardsProof{Proof: proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}}
	if !request.Budget.Spend() {
		return unknown
	}
	var guards []ResultGuard
	var named ssaflow.NamedResultCellsProof
	for instruction := range ssaflow.InstructionsWithin(function, request.Budget) {
		deferred, ok := instruction.(*ssa.Defer)
		if !ok {
			continue
		}
		closure, ok := deferred.Call.Value.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		cells := capturedResultCells(function, closure, request.Budget, &named)
		if request.Budget.Exhausted() || request.Budget.PoolExhausted() {
			return unknown
		}

		guard := ResultGuard{Defer: deferred, Cells: cells}
		if len(cells) != 0 && guard.turnsOnResult(request) {
			guards = append(guards, guard)
		}
		// Neither a missing guard nor a partial positive list is authoritative
		// when either the census or an opposing completion query stopped early.
		if request.Budget.Exhausted() || request.Budget.PoolExhausted() {
			return unknown
		}
	}
	if request.Budget.Exhausted() || request.Budget.PoolExhausted() {
		return unknown
	}
	return ResultGuardsProof{
		Proof:  proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceStructuralWalk, Provenance: proofs.EvidenceFromLocalSSA},
		Guards: guards,
	}
}

// One discovery owns the named-cell census. Captures retain their original
// order; repeated cells do not repeat the function search. Only a completed
// census may be reused, and its map says nothing about cleanup coverage.
func capturedResultCells(
	function *ssa.Function, closure *ssa.MakeClosure, budget *proofs.SearchBudget, named *ssaflow.NamedResultCellsProof,
) []*ssa.Alloc {
	var cells []*ssa.Alloc
	for _, binding := range closure.Bindings {
		if !budget.Spend() {
			return nil
		}
		cell, ok := binding.(*ssa.Alloc)
		if !ok {
			continue
		}
		if !named.Proven() {
			*named = ssaflow.ProveNamedResultCellsWithin(function, budget)
			if !named.Proven() {
				return nil
			}
		}
		if _, found := named.Cells[cell]; found {
			cells = append(cells, cell)
		}
	}
	return cells
}

func (guard ResultGuard) turnsOnResult(request CompletionRequest) bool {
	for _, cell := range guard.Cells {
		if !request.Budget.Spend() {
			return false
		}
		first, second := opposingOutcomes(cell)
		if first == ssacall.OutcomeAny {
			continue
		}
		one := guard.Completes(request, ssacall.FixedValues{cell: first})
		other := guard.Completes(request, ssacall.FixedValues{cell: second})
		if one == proofs.EvidenceProven && other == proofs.EvidenceDisproven ||
			one == proofs.EvidenceDisproven && other == proofs.EvidenceProven {
			return true
		}
	}
	return false
}

// opposingOutcomes returns the two outcomes a named result can be fixed to:
// nil and non-nil, or true and false.
func opposingOutcomes(cell *ssa.Alloc) (ssacall.Outcome, ssacall.Outcome) {
	pointer, ok := cell.Type().Underlying().(*types.Pointer)
	if !ok {
		return ssacall.OutcomeAny, ssacall.OutcomeAny
	}
	if ssacall.Nilable(pointer.Elem()) {
		return ssacall.OutcomeNil, ssacall.OutcomeNonNil
	}
	if basic, ok := pointer.Elem().Underlying().(*types.Basic); ok && basic.Info()&types.IsBoolean != 0 {
		return ssacall.OutcomeTrue, ssacall.OutcomeFalse
	}
	return ssacall.OutcomeAny, ssacall.OutcomeAny
}

// Completes asks whether the deferred literal completes the target on every
// one of its returns, given what its captured named results hold.
func (guard ResultGuard) Completes(request CompletionRequest, fixed ssacall.FixedValues) proofs.EvidenceState {
	request.Instruction, request.Coverage, request.Constants = guard.Defer, CoverageEveryReturn, fixed
	return ProveCompletion(request).State
}

// CompletesAtReturn asks whether the deferred literal completes the target
// when the function leaves through returned, which it must dominate. Each
// named result is fixed to the outcome outcomeOf gives the value the return
// stores; a value with no known outcome, or a result the return does not set
// itself, leaves the answer unknown. Cell binding and callback invocation
// spend request.Budget; callers also use it for outcome inference. A callback
// may not publish an outcome after exhausting that allowance.
func (guard ResultGuard) CompletesAtReturn(
	request CompletionRequest, returned *ssa.Return, outcomeOf func(ssa.Value) (ssacall.Outcome, bool),
) proofs.EvidenceState {
	fixed := ssacall.FixedValues{}
	for _, cell := range guard.Cells {
		if !request.Budget.Spend() {
			return proofs.EvidenceUnknown
		}
		value, ok := ssaflow.ValueAtReturnWithin(returned, cell, request.Budget)
		if !ok {
			return proofs.EvidenceUnknown
		}
		if !request.Budget.Spend() {
			return proofs.EvidenceUnknown
		}
		outcome, ok := outcomeOf(value)
		if !ok || request.Budget.Exhausted() || request.Budget.PoolExhausted() {
			return proofs.EvidenceUnknown
		}
		fixed[cell] = outcome
	}
	return guard.Completes(request, fixed)
}

// ProveReachesReturn distinguishes a defer registered on every path to
// returned from one that may reach it or is disconnected. Interrupted order
// or reachability searches remain unknown, never evidence of disconnection.
func (guard ResultGuard) ProveReachesReturn(returned *ssa.Return, budget *proofs.SearchBudget) proofs.Proof {
	state := proofs.EvidenceDisproven
	if cfg.InstructionDominatesWithin(guard.Defer, returned, budget) {
		state = proofs.EvidenceProven
	} else if cfg.InstructionMayFollowWithin(guard.Defer, returned, budget) {
		state = proofs.EvidenceUnknown
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}
	}
	return proofs.Proof{State: state, Reason: proofs.EvidenceStructuralWalk, Provenance: proofs.EvidenceFromLocalSSA}
}
