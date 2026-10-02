package resourcelifetime

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Ambiguous cleanup proves only that a possible acquisition origin reaches a
// cleanup receiver or a merged argument whose own cleanup is established.
// It cannot settle the caller's exact resource; cutoff supplies uncertainty
// rather than a completed absence of possible cleanup.
func (analysis *resourceAnalysis) proveAmbiguousCleanupWithin(
	instruction ssa.Instruction, common *ssa.CallCommon, budget *ssaflow.SearchBudget,
) resourceProof {
	if analysis.optional.Proven() || common == nil {
		return resourceProof{State: ssaflow.EvidenceDisproven, Reason: resourceReasonUntouched}
	}
	// A merged receiver or escaped owner projection may still select this
	// acquisition. Exact storage identity cannot establish that relationship,
	// but absence of a match is not proof that the resource stays open. A body
	// read helper can consume a response before its explicit Body.Close:
	// https://github.com/james-6-23/codex2api/blob/4f96afe95bb16132347f4ab74e63b0b1fa0f778b/auth/claude_api_key.go#L94-L99
	if slices.Contains(analysis.contract.cleanup, ssaflow.CallName(common)) {
		derived := heapmodel.ValueDerivesFromWithin(ssaflow.CallReceiver(common), analysis.resource, budget)
		proof := carriedValueProof(derived, resourceReasonAmbiguousCleanupValue, budget)
		if proof.State != ssaflow.EvidenceDisproven {
			return proof
		}
	}
	return analysis.proveAmbiguousHelperCleanupWithin(instruction, common, budget)
}

// A cleanup helper may receive a projection of a merged owner. Proving cleanup
// of its actual argument does not prove which acquisition was released, but
// that ambiguous identity cannot establish a leak either. Direct Close calls
// have the same unknown boundary above; read-only helpers do not qualify.
// https://github.com/mr-karan/doggo/blob/7f6b105f240e562a5c7659976b1c16b683c25385/pkg/resolvers/doh.go#L143-L168
func (analysis *resourceAnalysis) proveAmbiguousHelperCleanupWithin(
	instruction ssa.Instruction, common *ssa.CallCommon, budget *ssaflow.SearchBudget,
) resourceProof {
	for _, argument := range common.Args {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		if !mergedCleanupArgument(argument) {
			continue
		}
		derived := heapmodel.ValueDerivesFromWithin(argument, analysis.resource, budget)
		if resourceFlowExhausted(budget) {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		if !derived {
			continue
		}
		for _, method := range analysis.contract.cleanup {
			if !budget.Spend() {
				return carriedValueProof(false, resourceReasonUntouched, budget)
			}
			completion := lifecycle.CompletionRequest{
				Instruction: instruction, Target: argument, Methods: []string{method},
				Coverage: lifecycle.CoverageEveryReturn, Budget: budget,
			}
			proof := analysis.evidence.Prove(lifecyclefacts.EvidenceRequest{
				Instruction: instruction, Target: argument, Completion: &completion, SelectMask: releaseMask(instruction, argument, method),
			})
			if proof.Reason == ssaflow.EvidenceBudgetExhausted {
				return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
			}
			candidate := carriedValueProof(proof.Proven(), resourceReasonAmbiguousHelperCleanupValue, budget)
			if candidate.State != ssaflow.EvidenceDisproven {
				return candidate
			}
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

// Only merged values are ambiguous here. A helper that closes an overwritten
// field, or conditionally closes its exact argument, must remain diagnostic.
func mergedCleanupArgument(argument ssa.Value) bool {
	if _, merged := argument.(*ssa.Phi); merged {
		return true
	}
	load, loaded := argument.(*ssa.UnOp)
	if !loaded || load.Op != token.MUL {
		return false
	}
	field, projected := load.X.(*ssa.FieldAddr)
	if !projected {
		return false
	}
	_, merged := field.X.(*ssa.Phi)
	return merged
}
