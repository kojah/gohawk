package goroutineownership

import (
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/syntax"
	"golang.org/x/tools/go/ssa"
)

// Completion coverage proves the worker has no remaining work after a signal
// and that notifications cover returns. Its operation policy stays local;
// the shared obligation flow owns path coverage and budget uncertainty.

// terminalCompletion reports whether only returns can follow a completion
// operation. Later work cannot be joined by observing an earlier signal.
func terminalCompletion(done ssa.Instruction, budget *proofs.SearchBudget) bool {
	index := cfg.InstructionIndex(done)
	if index < 0 {
		return false
	}
	type cursor struct {
		block *ssa.BasicBlock
		index int
	}
	queue := []cursor{{block: done.Block(), index: index + 1}}
	seen := make(map[cursor]bool)
	for len(queue) > 0 {
		if !budget.Spend() {
			return false
		}
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			return false
		}
		seen[current] = true
		if current.index < len(current.block.Instrs) {
			switch current.block.Instrs[current.index].(type) {
			case *ssa.Return:
				continue
			case *ssa.RunDefers:
				if !ssacall.CallMatchesSymbol(ssaflow.InstructionCall(done), waitGroupDone) || !completionOnlyDefersWithin(done.Parent(), budget) {
					return false
				}
				queue = append(queue, cursor{block: current.block, index: current.index + 1})
				continue
			case *ssa.Jump:
				// Conditional terminal sends can jump to a shared return block.
				// A jump performs no work and preserves this completion proof.
			default:
				return false
			}
		}
		if len(current.block.Succs) == 0 {
			return false
		}
		for _, successor := range current.block.Succs {
			queue = append(queue, cursor{block: successor})
		}
	}
	return true
}

// A terminal Done followed only by other Done/close defers has finished the
// worker's actual work. Keeping that alternative handle does not treat an
// arbitrary deferred callback as complete: it may still block or mutate data.
// https://github.com/murphysecurity/murphysec/blob/59d5cdc9a53a9e7940250aa30ea4434d0e258c40/module/nuget/nuget_cmd_build.go#L611-L640
func completionOnlyDefersWithin(function *ssa.Function, budget *proofs.SearchBudget) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return false
			}
			deferred, ok := instruction.(*ssa.Defer)
			if !ok {
				continue
			}
			common := deferred.Common()
			if !ssacall.CallMatchesSymbol(common, waitGroupDone) &&
				!ssacall.CallMatchesSymbol(common, syntax.Builtin("close")) {
				return false
			}
		}
	}
	return true
}

// completionHasReturn requires a witness before coverage can create a promise.
func completionHasReturn(function *ssa.Function, budget *proofs.SearchBudget) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return false
			}
			if _, ok := instruction.(*ssa.Return); ok {
				return true
			}
		}
	}
	return false
}

// completionReturnCoverage charges both examined instructions and flow states.
// The supplied predicate owns operation policy; exhaustion remains uncertain.
func completionReturnCoverage(
	function *ssa.Function, nonNil ssa.Value, budget *proofs.SearchBudget, owns func(ssa.Instruction) bool,
) ssapath.ObligationOutcome {
	return ssapath.EvaluateObligationFromEntry(function, ssapath.ObligationFlow{
		Budget: budget, NonNil: nonNil,
		Instruction: func(instruction ssa.Instruction) ssapath.ObligationAction {
			if !budget.Spend() {
				return ssapath.ObligationUnknown
			}
			if owns(instruction) {
				return ssapath.ObligationExact
			}
			return ssapath.ObligationNone
		},
	})
}

// Unobserved completion signals require a complete channel-use census. An
// interrupted or temporally ambiguous alias search cannot establish that the
// parent has an observation protocol, so it cannot revive a missing-join claim.

func (analysis *spawnAnalysis) proveUnobservedSignalsWithin(budget *proofs.SearchBudget) proofs.Proof {
	disproven := proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound}
	unknown := proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}
	if len(analysis.signals) == 0 || len(analysis.groups) > 0 {
		return disproven
	}
	for _, signal := range analysis.signals {
		if !budget.Spend() {
			return unknown
		}
		made := localChannelWithin(analysis.function, signal, budget)
		if budget.Exhausted() || budget.PoolExhausted() {
			return unknown
		}
		if made == nil {
			return disproven
		}
		census := ssaflow.ProveChannelValuesWithin(made, budget)
		if !census.Proven() {
			return census.Proof
		}
		for _, use := range census.Uses {
			if !budget.Spend() {
				return unknown
			}
			if !ssacall.CallMatchesSymbol(ssaflow.InstructionCall(use.Instruction), syntax.Builtin("close")) {
				return disproven
			}
		}
	}
	return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceStructuralWalk, Provenance: proofs.EvidenceFromLocalSSA}
}
