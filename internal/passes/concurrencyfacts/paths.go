package concurrencyfacts

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Bounded paths preserve differing effects rather than merging them into one
// unconditional history. Correlations are deliberately over-approximated:
// consumers must prove a dependency on every path, including possible escapes.
// Loops, opaque instructions, and excessive products discard the whole answer.
const maxProtocolPaths = 8

func (engine *Engine) collectPaths(function *ssa.Function, root bool) Summary {
	if !trivialRecovery(function) {
		return Summary{Reason: "protocol-control-flow-unknown"}
	}
	order, reason := engine.orderedBlocks(function)
	if reason != "" {
		return Summary{Reason: reason}
	}
	states := map[*ssa.BasicBlock][]Summary{function.Blocks[0]: {{}}}
	var paths []Summary
	for _, block := range order {
		current := states[block]
		for _, instruction := range block.Instrs {
			current, reason = engine.advancePaths(current, instruction, root)
			if reason != "" {
				return Summary{Reason: reason}
			}
		}
		if len(block.Succs) == 0 {
			paths = append(paths, current...)
			if len(paths) > maxProtocolPaths {
				return Summary{Reason: "protocol-alternative-limit"}
			}
		}
		for _, next := range block.Succs {
			for _, state := range current {
				states[next] = append(states[next], cloneEffects(state))
			}
			if len(states[next]) > maxProtocolPaths {
				return Summary{Reason: "protocol-alternative-limit"}
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
		return Summary{Reason: "protocol-control-flow-unknown"}
	}
	for index := range paths {
		if len(paths[index].deferred) != 0 {
			return Summary{Reason: "protocol-deferred-effects-unknown"}
		}
		if paths[index].hasWorkerAlternatives() {
			paths[index].Reason = "protocol-select-alternatives"
			paths[index].AlternativesComplete = true
		}
		paths[index] = finishCancellation(paths[index])
	}
	return Summary{Paths: paths, Reason: "protocol-branch-alternatives"}
}

func (engine *Engine) advancePaths(states []Summary, instruction ssa.Instruction, root bool) ([]Summary, string) {
	var next []Summary
	for _, state := range states {
		if !engine.budget.Spend() {
			return nil, "protocol-budget-exhausted"
		}
		branches, reason, handled := engine.appendPathCall(state, instruction)
		if handled {
			if reason != "" {
				return nil, reason
			}
			next = append(next, branches...)
		} else {
			if reason := engine.appendInstruction(&state, instruction, root); reason != "" {
				return nil, reason
			}
			next = append(next, state)
		}
		if len(next) > maxProtocolPaths {
			return nil, "protocol-alternative-limit"
		}
	}
	for _, state := range next {
		if state.operationCount() > maxOperations {
			return nil, "protocol-summary-limit"
		}
	}
	return next, ""
}

// Splice a helper's complete alternatives into independent copies of the
// caller prefix. Never combine effects taken from different helper returns.
func (engine *Engine) appendPathCall(state Summary, instruction ssa.Instruction) ([]Summary, string, bool) {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return nil, "", false
	}
	called := engine.callSummary(call)
	if len(called.Paths) == 0 {
		return nil, "", false
	}
	var branches []Summary
	for _, path := range called.Paths {
		branch := cloneEffects(state)
		if reason := appendCalled(&branch, path, call); reason != "" {
			return nil, reason, true
		}
		branches = append(branches, branch)
	}
	return branches, "", true
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
	return Summary{Paths: result, Reason: "protocol-branch-alternatives"}
}
