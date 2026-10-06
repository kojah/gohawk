package goroutineownership

import (
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
)

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
			if !ssaflow.CallMatchesSymbol(ssaflow.InstructionCall(use.Instruction), syntax.Builtin("close")) {
				return disproven
			}
		}
	}
	return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceStructuralWalk, Provenance: proofs.EvidenceFromLocalSSA}
}
