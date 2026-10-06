package resourcelifetime

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/ssa"
)

// Return evidence distinguishes a possible owner handoff from an uncovered
// resource return. Result, derivation and ownership visits share the flow
// allowance, including wrapper decoding and view binding. Graph construction,
// alias-query internals and type queries retain independent costs. Cutoff
// cannot admit the uncovered-return witness.

type resourceReturnedWrapperProof struct {
	resourceProof
	Position int // Direct retaining result, or -1 for a nested or absent wrapper.
}

func (analysis *resourceAnalysis) returnedWrapperWithin(returned *ssa.Return, budget *proofs.SearchBudget) resourceReturnedWrapperProof {
	if proof, known := analysis.wrappers[returned]; known {
		return proof
	}
	proof := analysis.proveReturnedWrapperWithin(returned, budget)
	if proof.State == proofs.EvidenceUnknown {
		return proof
	}
	if analysis.wrappers == nil {
		analysis.wrappers = make(map[*ssa.Return]resourceReturnedWrapperProof)
	}
	analysis.wrappers[returned] = proof
	return proof
}

// proveReturnedWrapperWithin widens the possible handoff to a constructor
// result returned inside an aggregate. The constructor must dominate the
// return; a dropped wrapper on an earlier error path supplies no boundary.
// Positive evidence establishes possible retention only, so the classifier
// labels it unknown rather than settled. All census, dominance and containment
// visits share budget; graph and alias-query internals remain independent.
func (analysis *resourceAnalysis) proveReturnedWrapperWithin(returned *ssa.Return, budget *proofs.SearchBudget) resourceReturnedWrapperProof {
	position := analysis.returnedWrapperPositionWithin(returned, budget)
	found := position >= 0
	if !found && !resourceFlowExhausted(budget) {
		for instruction := range ssaflow.InstructionsWithin(analysis.function, budget) {
			call, ok := instruction.(*ssa.Call)
			if !ok || !cfg.InstructionDominatesWithin(call, returned, budget) ||
				!analysis.provenWrapperOfWithin(call, maxWrapperChain, budget) {
				continue
			}
			found = returnedContainsWrapperWithin(returned, call, budget)
			if found || resourceFlowExhausted(budget) {
				break
			}
		}
	}
	if resourceFlowExhausted(budget) {
		return resourceReturnedWrapperProof{
			resourceProof: resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}, Position: -1,
		}
	}
	if found {
		return resourceReturnedWrapperProof{
			resourceProof: resourceProof{State: proofs.EvidenceProven, Reason: resourceReasonReturnedWrapperRetains}, Position: position,
		}
	}
	return resourceReturnedWrapperProof{resourceProof: resourceProof{State: proofs.EvidenceDisproven}, Position: -1}
}

func returnedContainsWrapperWithin(returned *ssa.Return, call *ssa.Call, budget *proofs.SearchBudget) bool {
	for _, result := range returned.Results {
		if !budget.Spend() {
			return false
		}
		if lifecycle.ProveMayContainValueWithin(result, call, budget).Proven() {
			return true
		}
	}
	return false
}

func (analysis *resourceAnalysis) proveResourceReturn(returned *ssa.Return, budget *proofs.SearchBudget) resourceLifetimePolicyResult {
	owner := analysis.returnedResourceOwner(returned, budget)
	if owner.State == proofs.EvidenceUnknown {
		return unknownResourceLifetime(owner.Reason)
	}
	transferred := owner.State == proofs.EvidenceProven || heapmodel.ReturnedMayAliasAnyWithin(returned, analysis.owners, budget)
	if resourceFlowExhausted(budget) {
		return unknownResourceLifetime(resourceReasonBudgetExhausted)
	}
	if transferred {
		return acceptedResourceLifetime(resourceReasonReturnedMayTransfer)
	}
	return resourceLifetimePolicyResult{state: proofs.EvidenceProven, reason: resourceReasonUnownedReturn, leak: returned}
}

// returnedResourceOwner reports whether the return hands the resource to the
// caller through a value that can still release it. When a result derives
// from the resource but is declined, the trace says which rule declined it:
// a summarized view, or a projection with no cleanup method.
func (analysis *resourceAnalysis) returnedResourceOwner(returned *ssa.Return, budget *proofs.SearchBudget) resourceProof {
	resource := analysis.resource
	if !budget.Spend() {
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	if lifecycle.ProveReturnedOwnershipWithin(returned, resource, nil, budget).Proven() {
		return resourceProof{State: proofs.EvidenceProven, Reason: resourceReasonReturnedMayTransfer}
	}
	if resourceFlowExhausted(budget) {
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	wrapper := analysis.returnedWrapperWithin(returned, budget)
	if wrapper.State == proofs.EvidenceUnknown {
		return wrapper.resourceProof
	}
	if position := wrapper.Position; position >= 0 &&
		analysis.evidence.RetainingResultClaimed(analysis.function, position) {
		analysis.traceReturnedResult(returned, returned.Results[position], resourceReasonReturnedRetainingWrapper, analysisTrace.OutcomeAccepted)
		return resourceProof{State: proofs.EvidenceProven, Reason: resourceReasonReturnedMayTransfer}
	}
	if resourceFlowExhausted(budget) {
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	for _, result := range returned.Results {
		if !budget.Spend() {
			return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if !heapmodel.ValueDerivesFromWithin(result, resource, budget) {
			continue
		}
		proof := analysis.proveReturnedProjection(returned, result, budget)
		if proof.State == proofs.EvidenceUnknown || proof.Proven() {
			return proof
		}
	}
	if resourceFlowExhausted(budget) {
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	return resourceProof{State: proofs.EvidenceDisproven}
}

func (analysis *resourceAnalysis) proveReturnedProjection(returned *ssa.Return, result ssa.Value, budget *proofs.SearchBudget) resourceProof {
	resource, cleanup := analysis.resource, analysis.contract.cleanup
	// Narrowing an interface preserves its dynamic object, including Close:
	// the caller can still recover io.Closer by assertion. Require the
	// unchanged cleanup-bearing projection, not a transformed reader or a
	// replacement body that merely occupies the original field.
	// https://github.com/lich0821/ccNexus/blob/55887d232555f94ea4db621a5a7e65430eebf0d7/internal/transformer/tool_chain.go#L121-L135
	if !budget.Spend() {
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	if original, changed := ssaflow.UnwrapTransparentValue(result, ssaflow.TransparentChangeInterface); changed {
		projectionBudget := budget.Within(proofs.QueryBudget)
		projection := heapmodel.NewStorage(projectionBudget).Projection(original, resource, returned)
		if projectionBudget.Exhausted() || projectionBudget.PoolExhausted() {
			return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if projection.Proven() {
			result = original
		}
	}
	// A returned view is summarized as releasing nothing, whatever its
	// method names suggest; the caller of this function cannot close the
	// resource through it.
	if resourceFlowExhausted(budget) {
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	if call, ok := result.(*ssa.Call); ok {
		view := analysis.summaries.ProveCallReturnsViewWithin(call, resource, budget)
		if view.State == proofs.EvidenceUnknown && view.Reason == proofs.EvidenceBudgetExhausted {
			return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if view.Proven() {
			analysis.traceReturnedResult(returned, result, resourceReasonReturnedViewCannotRelease, analysisTrace.OutcomeRejected)
			return resourceProof{State: proofs.EvidenceDisproven}
		}
	}
	methods := types.NewMethodSet(result.Type())
	for method := range methods.Methods() {
		if !budget.Spend() {
			return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if slices.Contains(cleanup, method.Obj().Name()) {
			analysis.traceReturnedResult(returned, result, resourceReasonReturnedCleanupProjection, analysisTrace.OutcomeAccepted)
			return resourceProof{State: proofs.EvidenceProven, Reason: resourceReasonReturnedMayTransfer}
		}
	}
	analysis.traceReturnedResult(returned, result, resourceReasonReturnedProjectionLacksCleanup, analysisTrace.OutcomeRejected)
	return resourceProof{State: proofs.EvidenceDisproven}
}

// returnedWrapperPositionWithin reports the result position at which a return hands
// back a chain of wrappers over the resource, each proven by its summary to
// hold its argument on every return, as slog.New(slog.NewTextHandler(file,
// nil)) holds the file. The result has no method that releases the resource,
// but the caller receives it and can keep it for as long as it needs the
// wrapper. When the constructor's own summary claims that result as a
// retaining result, the caller owes the obligation and the return is a
// handover; otherwise the chain is only an uncertain boundary. A later error
// return that discards the wrapper still abandons the resource. A may-hold
// wrapper, such as bufio.NewWriter, is not a chain step and stays reported.
// https://github.com/datolabs-io/opsy/blob/8c588e1c17da76db92351ccaf9b1fdd5793ab5f5/internal/config/config.go#L186-L209
func (analysis *resourceAnalysis) returnedWrapperPositionWithin(returned *ssa.Return, budget *proofs.SearchBudget) int {
	for position, result := range returned.Results {
		if !budget.Spend() {
			return -1
		}
		if analysis.provenWrapperOfWithin(result, maxWrapperChain, budget) && !resourceFlowExhausted(budget) {
			return position
		}
	}
	return -1
}

func (analysis *resourceAnalysis) provenWrapperOfWithin(value ssa.Value, depth int, budget *proofs.SearchBudget) bool {
	if !budget.Spend() {
		return false
	}
	call, ok := unwrapWrapperWithin(value, budget).(*ssa.Call)
	if !ok || depth == 0 {
		return false
	}
	for index, argument := range call.Common().Args {
		if !budget.Spend() {
			return false
		}
		inner := unwrapWrapperWithin(argument, budget)
		if resourceFlowExhausted(budget) {
			return false
		}
		if !heapmodel.MayAlias(inner, analysis.resource) && !analysis.provenWrapperOfWithin(inner, depth-1, budget) {
			continue
		}
		if !budget.Spend() {
			return false
		}
		if owner, _ := analysis.evidence.CalleeClaims(call, index, lifecyclefacts.ClaimReturnsOwner); owner && !resourceFlowExhausted(budget) {
			return true
		}
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
