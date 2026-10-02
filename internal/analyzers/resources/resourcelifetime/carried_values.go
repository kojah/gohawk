package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Carried-value evidence distinguishes a direct argument from a nested value.
// Both can supply opaque consumption, never exact cleanup. Structural visits
// share caller allowance; alias/graph/type and effect internals retain
// independent costs.

func (analysis *resourceAnalysis) carriedPayload(value ssa.Value, reason resourceLifetimeReason) (resourceLifetimeReason, bool) {
	proof := analysis.proveCarriedValueWithin(value, analysis.budget(ssaflow.SummaryBudget))
	if proof.State == ssaflow.EvidenceUnknown {
		return proof.Reason, true
	}
	return reason, proof.Proven()
}

func (analysis *resourceAnalysis) proveCarriedArgumentsWithin(common *ssa.CallCommon, budget *ssaflow.SearchBudget) resourceProof {
	for _, argument := range common.Args {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		proof := analysis.proveCarriedValueWithin(argument, budget)
		if proof.State != ssaflow.EvidenceDisproven {
			return proof
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

// A struct literal wrapping a type-asserted response body and passed to a
// function value may carry the resource; kandev's SPDY handoff is such a case:
// https://github.com/kdlbs/kandev/blob/17da0aafe33df01828e21fc79cc9dd156dc088dc/apps/backend/internal/agent/kubernetes/portforward.go#L464-L491
func (analysis *resourceAnalysis) proveCarriedValueWithin(value ssa.Value, budget *ssaflow.SearchBudget) resourceProof {
	direct := analysis.proveCarriedDirectlyWithin(value, budget)
	if direct.State != ssaflow.EvidenceDisproven {
		return direct
	}
	within := analysis.proveNestedCarryWithin(value, budget)
	if within.State != ssaflow.EvidenceDisproven {
		return within
	}
	return analysis.provePossibleWrapperWithin(value, 0, false, budget)
}

func (analysis *resourceAnalysis) proveCarriedDirectlyWithin(value ssa.Value, budget *ssaflow.SearchBudget) resourceProof {
	if !budget.Spend() {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	if heapmodel.MayAlias(value, analysis.resource) || heapmodel.ValueDerivesFromWithin(value, analysis.resource, budget) {
		return carriedValueProof(true, resourceReasonDirectMayCarry, budget)
	}
	if resourceFlowExhausted(budget) {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	// Stable local reads can name the resource directly. Preserve the existing
	// storage cap, but a bounded caller must distinguish cutoff from no match.
	storageBudget := analysis.directStorageBudget(budget)
	same := heapmodel.NewStorage(storageBudget).Same(value, analysis.resource)
	if budget != nil && resourceFlowExhausted(storageBudget) {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	return carriedValueProof(same.Proven(), resourceReasonDirectMayCarry, budget)
}

func (analysis *resourceAnalysis) directStorageBudget(budget *ssaflow.SearchBudget) *ssaflow.SearchBudget {
	if budget == nil {
		return analysis.budget(ssaflow.QueryBudget)
	}
	return budget.Within(ssaflow.QueryBudget)
}

func (analysis *resourceAnalysis) proveNestedCarryWithin(value ssa.Value, budget *ssaflow.SearchBudget) resourceProof {
	contained := lifecycle.ProveMayContainValueWithin(value, analysis.resource, budget)
	if contained.State == ssaflow.EvidenceUnknown {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	if contained.Proven() {
		return carriedValueProof(true, resourceReasonAggregateMayCarry, budget)
	}
	// This fallback asks about derived stored values, beyond object containment.
	// Keep it analyzer-owned and preserve the exact transparent forms and policy.
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface
	found := ssaflow.NewReachingWalk(forms).Within(budget).Any(value, func(_ ssaflow.ReachingWalk, value ssa.Value) bool {
		if _, ok := value.(*ssa.Alloc); !ok {
			return false
		}
		for stored := range lifecycle.StoredIntoWithin(value, budget) {
			if heapmodel.ValueDerivesFromWithin(stored, analysis.resource, budget) {
				return true
			}
		}
		return false
	})
	return carriedValueProof(found, resourceReasonAggregateMayCarry, budget)
}

func carriedValueProof(found bool, reason resourceLifetimeReason, budget *ssaflow.SearchBudget) resourceProof {
	if resourceFlowExhausted(budget) {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	if found {
		return resourceProof{State: ssaflow.EvidenceProven, Reason: reason}
	}
	return resourceProof{State: ssaflow.EvidenceDisproven, Reason: resourceReasonUntouched}
}
