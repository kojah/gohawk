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
		engine.recordBlockCutoff(function.Recover, cutoffRecovery)
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
			if reason == "protocol-select-alternatives" {
				state.Reason = reason
				return state
			}
			return Summary{Reason: reason}
		}
		if len(block.Succs) == 0 {
			if len(state.deferred) != 0 {
				return Summary{Reason: "protocol-deferred-effects-unknown"}
			}
			if terminal != nil && !sameEffects(*terminal, state) {
				engine.recordBlockCutoff(block, cutoffBranch)
				return Summary{Reason: "protocol-branch-effects-differ"}
			}
			terminal = &state
		}
		for _, next := range block.Succs {
			// A join with different synchronization histories is not one
			// unconditional protocol, even if later operations happen to agree.
			if previous, exists := states[next]; exists && !sameEffects(previous, state) {
				engine.recordBlockCutoff(block, cutoffBranch)
				return Summary{Reason: "protocol-branch-effects-differ"}
			}
			states[next] = cloneEffects(state)
		}
	}
	if terminal == nil {
		return Summary{Reason: "protocol-control-flow-unknown"}
	}
	if terminal.hasWorkerAlternatives() {
		// Only a fully visited caller may turn a worker's complete arms into
		// graph variants. An early bailout retains choices but no proof paths.
		terminal.Reason = "protocol-select-alternatives"
		terminal.AlternativesComplete = true
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
			engine.recordBlockCutoff(order[index], cutoffBranch)
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
		for _, block := range function.Blocks {
			if pending[block] > 0 {
				engine.recordBlockCutoff(block, cutoffLoop)
				break
			}
		}
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
		return a.Spawn == b.Spawn && a.Site == b.Site && a.Prefix == b.Prefix && a.Branches == b.Branches &&
			slices.EqualFunc(a.Operations, b.Operations, sameOperation) &&
			slices.EqualFunc(a.Alternatives, b.Alternatives, func(x, y []Operation) bool {
				return slices.EqualFunc(x, y, sameOperation)
			})
	}
	return slices.EqualFunc(first.Operations, second.Operations, sameOperation) &&
		slices.Equal(first.CancellationInputs, second.CancellationInputs) &&
		slices.EqualFunc(first.Workers, second.Workers, sameWorker) &&
		slices.EqualFunc(first.deferred, second.deferred, sameOperation)
}

func cloneEffects(summary Summary) Summary {
	summary.CancellationInputs = slices.Clone(summary.CancellationInputs)
	summary.Operations = slices.Clone(summary.Operations)
	summary.Workers = slices.Clone(summary.Workers)
	for index := range summary.Workers {
		summary.Workers[index].Operations = slices.Clone(summary.Workers[index].Operations)
		summary.Workers[index].Alternatives = slices.Clone(summary.Workers[index].Alternatives)
		for arm := range summary.Workers[index].Alternatives {
			summary.Workers[index].Alternatives[arm] = slices.Clone(summary.Workers[index].Alternatives[arm])
		}
	}
	summary.deferred = slices.Clone(summary.deferred)
	summary.Choices = slices.Clone(summary.Choices)
	for index := range summary.Choices {
		summary.Choices[index].Arms = slices.Clone(summary.Choices[index].Arms)
		for arm := range summary.Choices[index].Arms {
			summary.Choices[index].Arms[arm].Sequence = slices.Clone(summary.Choices[index].Arms[arm].Sequence)
		}
	}
	return summary
}
