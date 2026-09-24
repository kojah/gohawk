package concurrencyfacts

import (
	"slices"

	"golang.org/x/tools/go/ssa"
)

// Acyclic control flow is folded once per block. Joins require identical
// prefixes, including pending defers and every worker launch. We intentionally
// decline paths that could only agree after cancellation of earlier effects.
// The fixed sequence limit bounds copying; no execution paths are enumerated.
func (engine *Engine) collectBranches(function *ssa.Function, root bool) Summary {
	if !trivialRecovery(function) {
		return Summary{Reason: "protocol-control-flow-unknown"}
	}
	order, reason := engine.orderedBlocks(function)
	if reason != "" {
		return Summary{Reason: reason}
	}
	states := map[*ssa.BasicBlock]Summary{function.Blocks[0]: {}}
	var terminal *Summary
	for _, block := range order {
		state := states[block]
		if reason := engine.collectBlock(&state, block, root); reason != "" {
			return Summary{Reason: reason}
		}
		if len(block.Succs) == 0 {
			if len(state.deferred) != 0 {
				return Summary{Reason: "protocol-deferred-effects-unknown"}
			}
			if terminal != nil && !sameEffects(*terminal, state) {
				return Summary{Reason: "protocol-branch-effects-differ"}
			}
			terminal = &state
		}
		for _, next := range block.Succs {
			if previous, exists := states[next]; exists && !sameEffects(previous, state) {
				return Summary{Reason: "protocol-branch-effects-differ"}
			}
			states[next] = cloneEffects(state)
		}
	}
	if terminal == nil {
		return Summary{Reason: "protocol-control-flow-unknown"}
	}
	return *terminal
}

func (engine *Engine) orderedBlocks(function *ssa.Function) ([]*ssa.BasicBlock, string) {
	// Kahn's order visits each edge once and leaves cycles unprocessed.
	pending := make(map[*ssa.BasicBlock]int, len(function.Blocks))
	for _, block := range function.Blocks {
		if block != function.Recover {
			pending[block] = len(block.Preds)
		}
	}
	order := []*ssa.BasicBlock{function.Blocks[0]}
	for index := 0; index < len(order); index++ {
		if !engine.budget.Spend() {
			return nil, "protocol-budget-exhausted"
		}
		for _, next := range order[index].Succs {
			pending[next]--
			if pending[next] == 0 {
				order = append(order, next)
			}
		}
	}
	if len(order) != len(pending) {
		return nil, "protocol-control-flow-unknown"
	}
	return order, ""
}

func trivialRecovery(function *ssa.Function) bool {
	if function.Recover == nil {
		return true
	}
	recovery := function.Recover
	if len(recovery.Preds) != 0 || len(recovery.Instrs) != 1 {
		return false
	}
	returned, ok := recovery.Instrs[0].(*ssa.Return)
	return ok && len(returned.Results) == 0
}

func sameEffects(first, second Summary) bool {
	sameOperation := func(a, b Operation) bool { return a.Kind == b.Kind && a.Resource == b.Resource }
	sameWorker := func(a, b WorkerSummary) bool {
		return a.Spawn == b.Spawn && a.Prefix == b.Prefix && slices.EqualFunc(a.Operations, b.Operations, sameOperation)
	}
	return slices.EqualFunc(first.Operations, second.Operations, sameOperation) &&
		slices.EqualFunc(first.Workers, second.Workers, sameWorker) &&
		slices.EqualFunc(first.deferred, second.deferred, sameOperation)
}

func cloneEffects(summary Summary) Summary {
	summary.Operations = slices.Clone(summary.Operations)
	summary.Workers = slices.Clone(summary.Workers)
	for index := range summary.Workers {
		summary.Workers[index].Operations = slices.Clone(summary.Workers[index].Operations)
	}
	summary.deferred = slices.Clone(summary.deferred)
	return summary
}
