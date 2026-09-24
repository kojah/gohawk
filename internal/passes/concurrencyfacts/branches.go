package concurrencyfacts

import (
	"go/token"
	"slices"

	"golang.org/x/tools/go/ssa"
)

// Acyclic control flow is folded once per block. Joins require identical
// prefixes, including pending defers and every worker launch. We intentionally
// decline paths that could only agree after cancellation of earlier effects.
// The fixed sequence limit bounds copying; no execution paths are enumerated.
func (engine *Engine) collectBranches(function *ssa.Function, root bool) Summary {
	if !detachedRecovery(function) {
		engine.recordBlockCutoff(function.Recover, cutoffRecovery)
		return Summary{Reason: ReasonControlFlowUnknown}
	}
	flow, reason := engine.orderedBlocks(function, root)
	if reason != ReasonNone {
		return Summary{Reason: reason}
	}
	states := map[*ssa.BasicBlock]Summary{function.Blocks[0]: {}}
	var terminal *Summary
	for _, block := range flow.order {
		if panics(block) {
			continue
		}
		state := states[block]
		collect := engine.collectBlock
		if flow.isFolded(block) {
			// A folded loop adds its replay, if any; see loops.go.
			collect = func(state *Summary, _ *ssa.BasicBlock, root bool) Reason {
				return engine.replayLoop(state, flow.folded[block], root)
			}
		}
		if reason := collect(&state, block, root); reason != ReasonNone {
			if reason == ReasonSelectAlternatives {
				state.Reason = reason
				return state
			}
			return Summary{Reason: reason}
		}
		successors := flow.successors(block)
		if len(successors) == 0 {
			if len(state.deferred) != 0 {
				return Summary{Reason: ReasonDeferredEffectsUnknown}
			}
			if terminal != nil && !sameEffects(*terminal, state) {
				engine.recordBlockCutoff(block, cutoffBranch)
				return Summary{Reason: ReasonBranchEffectsDiffer}
			}
			if terminal != nil {
				state = foldSources(state, *terminal)
			}
			terminal = &state
		}
		for _, next := range successors {
			// A join with different synchronization histories is not one
			// unconditional protocol, even if later operations happen to agree.
			previous, exists := states[next]
			if exists && !sameEffects(previous, state) {
				engine.recordBlockCutoff(block, cutoffBranch)
				return Summary{Reason: ReasonBranchEffectsDiffer}
			}
			states[next] = cloneEffects(state)
			if exists {
				states[next] = foldSources(states[next], previous)
			}
		}
	}
	if terminal == nil {
		return Summary{Reason: ReasonControlFlowUnknown}
	}
	if terminal.hasWorkerAlternatives() {
		// Only a fully visited caller may turn a worker's complete arms into
		// graph variants. An early bailout retains choices but no proof paths.
		terminal.Reason = ReasonSelectAlternatives
		terminal.AlternativesComplete = true
	}
	return *terminal
}

func (engine *Engine) orderedBlocks(function *ssa.Function, root bool) (acyclicFlow, Reason) {
	flow := acyclicFlow{folded: engine.foldLoops(function, root)}
	inside := make(map[*ssa.BasicBlock]bool)
	for header, folded := range flow.folded {
		for _, block := range folded.loop.Blocks {
			inside[block] = block != header
		}
	}
	// Kahn's order visits each edge once and leaves cycles unprocessed.
	pending := make(map[*ssa.BasicBlock]int, len(function.Blocks))
	for _, block := range function.Blocks {
		if block != function.Recover && !inside[block] {
			pending[block] += 0
			for _, next := range flow.successors(block) {
				pending[next]++
			}
		}
	}
	flow.order = []*ssa.BasicBlock{function.Blocks[0]}
	for index := 0; index < len(flow.order); index++ {
		if !engine.budget.Spend() {
			engine.recordBlockCutoff(flow.order[index], cutoffBranch)
			return acyclicFlow{}, ReasonBudgetExhausted
		}
		for _, next := range flow.successors(flow.order[index]) {
			pending[next]--
			if pending[next] == 0 {
				flow.order = append(flow.order, next)
			}
		}
	}
	if len(flow.order) != len(pending) {
		for _, block := range function.Blocks {
			if pending[block] > 0 {
				engine.recordBlockCutoff(block, cutoffLoop)
				break
			}
		}
		return acyclicFlow{}, ReasonControlFlowUnknown
	}
	return flow, ReasonNone
}

// SSA gives every function with a defer a detached recovery block that reloads
// the results and returns them. The runtime enters it only when a deferred
// call stops a panic with recover. The collectors never visit it, because a
// completed summary admits only deferred calls whose complete summaries are
// releases (deferCompletion), and a call to recover leaves a summary incomplete.
// So the block is dead whenever the rest of the summary succeeds, and its
// result loads are not effects. A block with predecessors is not that shape.
func detachedRecovery(function *ssa.Function) bool {
	return function.Recover == nil || len(function.Recover.Preds) == 0
}

func sameEffects(first, second Summary) bool {
	sameOperation := func(a, b Operation) bool { return a.Kind == b.Kind && a.Resource == b.Resource }
	sameWorker := func(a, b WorkerSummary) bool {
		return a.Spawn == b.Spawn && a.Site == b.Site && a.Prefix == b.Prefix && a.Branches == b.Branches && a.Replicated == b.Replicated &&
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
	summary.Conditions = slices.Clone(summary.Conditions)
	summary.Returned = slices.Clone(summary.Returned)
	summary.Operations = slices.Clone(summary.Operations)
	summary.Workers = slices.Clone(summary.Workers)
	for index := range summary.Workers {
		summary.Workers[index].Operations = slices.Clone(summary.Workers[index].Operations)
		summary.Workers[index].Alternatives = slices.Clone(summary.Workers[index].Alternatives)
		summary.Workers[index].AlternativeConditions = slices.Clone(summary.Workers[index].AlternativeConditions)
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

// foldSources keeps the positions of an equal branch that is being merged
// away, so consumers can still attribute each branch's operation. It assumes
// sameEffects already matched the two operation sequences.
func foldSources(into, from Summary) Summary {
	into.Operations = slices.Clone(into.Operations)
	for index, operation := range from.Operations {
		target := &into.Operations[index]
		sources := append([]token.Pos{operation.Source}, operation.Alternates...)
		target.Alternates = slices.Clone(target.Alternates)
		for _, source := range sources {
			if source != target.Source && !slices.Contains(target.Alternates, source) {
				target.Alternates = append(target.Alternates, source)
			}
		}
	}
	return into
}

// A block that ends in panic never returns normally. A function that recovers
// is never complete (see detachedRecovery), so the panic ends the path before
// any later event, and the path cannot disagree with the others about the
// ordered effects that follow. If every path panics, no terminal remains and
// the summary stays unknown.
func panics(block *ssa.BasicBlock) bool {
	if len(block.Instrs) == 0 {
		return false
	}
	_, ok := block.Instrs[len(block.Instrs)-1].(*ssa.Panic)
	return ok
}
