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
// resource return. Result/derivation visits share the flow allowance; recursive
// ownership, wrapper decoding, graph construction and type queries retain
// independent costs. Cutoff cannot admit the uncovered-return witness.

func (analysis *resourceAnalysis) proveResourceReturn(returned *ssa.Return, budget *ssaflow.SearchBudget) resourceLifetimePolicyResult {
	transferred := analysis.returnedResourceOwner(returned, budget) || heapmodel.ReturnedMayAliasAnyWithin(returned, analysis.owners, budget)
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
func (analysis *resourceAnalysis) returnedResourceOwner(returned *ssa.Return, budget *ssaflow.SearchBudget) bool {
	resource, cleanup := analysis.resource, analysis.contract.cleanup
	if !budget.Spend() {
		return false
	}
	if lifecycle.ReturnedValueOwnsValue(returned, resource) {
		return true
	}
	if position := analysis.returnedWrapperPositionWithin(returned, budget); position >= 0 &&
		analysis.evidence.RetainingResultClaimed(analysis.function, position) {
		analysis.traceReturnedResult(returned, returned.Results[position], resourceReasonReturnedRetainingWrapper, analysisTrace.OutcomeAccepted)
		return true
	}
	for _, result := range returned.Results {
		if !budget.Spend() {
			return false
		}
		if !heapmodel.ValueDerivesFromWithin(result, resource, budget) {
			continue
		}
		// Narrowing an interface preserves its dynamic object, including Close:
		// the caller can still recover io.Closer by assertion. Require the
		// unchanged cleanup-bearing projection, not a transformed reader or a
		// replacement body that merely occupies the original field.
		// https://github.com/lich0821/ccNexus/blob/55887d232555f94ea4db621a5a7e65430eebf0d7/internal/transformer/tool_chain.go#L121-L135
		if original, changed := ssaflow.UnwrapTransparentValue(result, ssaflow.TransparentChangeInterface); changed &&
			heapmodel.NewStorage(analysis.budget(1000)).Projection(original, resource, returned).Proven() {
			result = original
		}
		// A returned view is summarized as releasing nothing, whatever its
		// method names suggest; the caller of this function cannot close the
		// resource through it.
		if call, ok := result.(*ssa.Call); ok && resourceSummaries.Provider(analysis.pass).CallReturnsView(call, resource) {
			analysis.traceReturnedResult(returned, result, resourceReasonReturnedViewCannotRelease, analysisTrace.OutcomeRejected)
			continue
		}
		methods := types.NewMethodSet(result.Type())
		for method := range methods.Methods() {
			if !budget.Spend() {
				return false
			}
			if slices.Contains(cleanup, method.Obj().Name()) {
				analysis.traceReturnedResult(returned, result, resourceReasonReturnedCleanupProjection, analysisTrace.OutcomeAccepted)
				return true
			}
		}
		analysis.traceReturnedResult(returned, result, resourceReasonReturnedProjectionLacksCleanup, analysisTrace.OutcomeRejected)
	}
	return false
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
