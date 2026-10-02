package resourcelifetime

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/ssa"
)

// Return evidence distinguishes a possible owner handoff from an uncovered
// resource return. Result, derivation and ownership visits share the flow
// allowance, including wrapper decoding and view binding. Graph construction,
// alias-query internals and type queries retain independent costs. Cutoff
// cannot admit the uncovered-return witness.

func (analysis *resourceAnalysis) proveResourceReturn(returned *ssa.Return, budget *ssaflow.SearchBudget) resourceLifetimePolicyResult {
	owner := analysis.returnedResourceOwner(returned, budget)
	if owner.State == ssaflow.EvidenceUnknown {
		return unknownResourceLifetime(owner.Reason)
	}
	transferred := owner.State == ssaflow.EvidenceProven || heapmodel.ReturnedMayAliasAnyWithin(returned, analysis.owners, budget)
	if resourceFlowExhausted(budget) {
		return unknownResourceLifetime(resourceReasonBudgetExhausted)
	}
	if transferred {
		return acceptedResourceLifetime(resourceReasonReturnedMayTransfer)
	}
	return resourceLifetimePolicyResult{state: ssaflow.EvidenceProven, reason: resourceReasonUnownedReturn, leak: returned}
}

// returnedResourceOwner reports whether the return hands the resource to the
// caller through a value that can still release it. When a result derives
// from the resource but is declined, the trace says which rule declined it:
// a summarized view, or a projection with no cleanup method.
func (analysis *resourceAnalysis) returnedResourceOwner(returned *ssa.Return, budget *ssaflow.SearchBudget) resourceProof {
	resource := analysis.resource
	if !budget.Spend() {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	if lifecycle.ProveReturnedOwnershipWithin(returned, resource, nil, budget).Proven() {
		return resourceProof{State: ssaflow.EvidenceProven, Reason: resourceReasonReturnedMayTransfer}
	}
	if position := analysis.returnedWrapperPositionWithin(returned, budget); position >= 0 &&
		analysis.evidence.RetainingResultClaimed(analysis.function, position) {
		analysis.traceReturnedResult(returned, returned.Results[position], resourceReasonReturnedRetainingWrapper, analysisTrace.OutcomeAccepted)
		return resourceProof{State: ssaflow.EvidenceProven, Reason: resourceReasonReturnedMayTransfer}
	}
	if resourceFlowExhausted(budget) {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	for _, result := range returned.Results {
		if !budget.Spend() {
			return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if !heapmodel.ValueDerivesFromWithin(result, resource, budget) {
			continue
		}
		proof := analysis.proveReturnedProjection(returned, result, budget)
		if proof.State == ssaflow.EvidenceUnknown || proof.Proven() {
			return proof
		}
	}
	if resourceFlowExhausted(budget) {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	return resourceProof{State: ssaflow.EvidenceDisproven}
}

func (analysis *resourceAnalysis) proveReturnedProjection(returned *ssa.Return, result ssa.Value, budget *ssaflow.SearchBudget) resourceProof {
	resource, cleanup := analysis.resource, analysis.contract.cleanup
	// Narrowing an interface preserves its dynamic object, including Close:
	// the caller can still recover io.Closer by assertion. Require the
	// unchanged cleanup-bearing projection, not a transformed reader or a
	// replacement body that merely occupies the original field.
	// https://github.com/lich0821/ccNexus/blob/55887d232555f94ea4db621a5a7e65430eebf0d7/internal/transformer/tool_chain.go#L121-L135
	if !budget.Spend() {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	if original, changed := ssaflow.UnwrapTransparentValue(result, ssaflow.TransparentChangeInterface); changed {
		projectionBudget := budget.Within(ssaflow.QueryBudget)
		projection := heapmodel.NewStorage(projectionBudget).Projection(original, resource, returned)
		if projectionBudget.Exhausted() || projectionBudget.PoolExhausted() {
			return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if projection.Proven() {
			result = original
		}
	}
	// A returned view is summarized as releasing nothing, whatever its
	// method names suggest; the caller of this function cannot close the
	// resource through it.
	if resourceFlowExhausted(budget) {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	if call, ok := result.(*ssa.Call); ok {
		view := analysis.summaries.ProveCallReturnsViewWithin(call, resource, budget)
		if view.State == ssaflow.EvidenceUnknown && view.Reason == ssaflow.EvidenceBudgetExhausted {
			return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if view.Proven() {
			analysis.traceReturnedResult(returned, result, resourceReasonReturnedViewCannotRelease, analysisTrace.OutcomeRejected)
			return resourceProof{State: ssaflow.EvidenceDisproven}
		}
	}
	methods := types.NewMethodSet(result.Type())
	for method := range methods.Methods() {
		if !budget.Spend() {
			return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if slices.Contains(cleanup, method.Obj().Name()) {
			analysis.traceReturnedResult(returned, result, resourceReasonReturnedCleanupProjection, analysisTrace.OutcomeAccepted)
			return resourceProof{State: ssaflow.EvidenceProven, Reason: resourceReasonReturnedMayTransfer}
		}
	}
	analysis.traceReturnedResult(returned, result, resourceReasonReturnedProjectionLacksCleanup, analysisTrace.OutcomeRejected)
	return resourceProof{State: ssaflow.EvidenceDisproven}
}

func (analysis *resourceAnalysis) traceReturnedResult(returned *ssa.Return, result ssa.Value, reason resourceLifetimeReason, outcome analysisTrace.Outcome) {
	if !analysis.probe.Enabled() {
		return
	}
	step := analysisTrace.Step{
		Reason: reason.String(), Outcome: outcome, Pos: returned.Pos(), Function: returned.Parent().String(),
		Details: map[string]string{"result": result.Name(), "result_type": result.Type().String()},
	}
	if outcome == analysisTrace.OutcomeAccepted {
		analysis.probe.Evidence(step)
		return
	}
	analysis.probe.Considered(step)
}
