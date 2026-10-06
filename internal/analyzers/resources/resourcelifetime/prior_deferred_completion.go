package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Prior deferred completion asks the ordinary evidence engine with anywhere
// coverage. A complete witness supplies only may-release uncertainty; this
// census never settles an acquisition or duplicates the resource flow.

// A defer registered on every path to acquisition may drain a captured closer
// slice populated later. The deferred body decides at return how many entries
// it closes, so requiring every-return cleanup would lose this uncertainty.
// https://github.com/bazel-contrib/rules_img/blob/af5e1452f0cb68b1ed64dc6095210f1eb4ae625f/img_tool/cmd/mtree/mtree.go#L110-L128
func (analysis *resourceAnalysis) proveDeferredBeforeAcquisitionWithin(call *ssa.Call, budget *proofs.SearchBudget) resourceProof {
	for instruction := range ssaflow.InstructionsWithin(call.Parent(), budget) {
		deferred, ok := instruction.(*ssa.Defer)
		if !ok || !ssaflow.InstructionDominates(deferred, call) {
			continue
		}
		completion := lifecycle.CompletionRequest{
			Instruction: deferred, Target: analysis.resource, Methods: analysis.contract.cleanup,
			Coverage: lifecycle.CoverageAnywhere, Budget: budget,
		}
		proof := analysis.evidence.Prove(lifecyclefacts.EvidenceRequest{Instruction: deferred, Target: analysis.resource, Completion: &completion})
		action, reason := releaseLabel(proof)
		// An interrupted nested search cannot complete a negative census even
		// when the caller pool still has room. Other completed unavailable forms
		// retain the prior model's policy and may be recognized by later classifiers.
		if reason == resourceReasonBudgetExhausted || resourceFlowExhausted(budget) {
			return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if action == actionSettled {
			return carriedValueProof(true, resourceReasonPriorDeferMayRelease, budget)
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}
