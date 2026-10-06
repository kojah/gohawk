package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

// Carried-value evidence distinguishes a direct argument from a nested value.
// Both can supply opaque consumption, never exact cleanup. Structural visits
// share caller allowance; alias/graph/type and effect internals retain
// independent costs.

func (analysis *resourceAnalysis) carriedPayload(value ssa.Value, reason resourceLifetimeReason) (resourceLifetimeReason, bool) {
	proof := analysis.proveCarriedValueWithin(value, analysis.budget(proofs.SummaryBudget))
	if proof.State == proofs.EvidenceUnknown {
		return proof.Reason, true
	}
	return reason, proof.Proven()
}

func (analysis *resourceAnalysis) proveCarriedArgumentsWithin(common *ssa.CallCommon, budget *proofs.SearchBudget) resourceProof {
	for _, argument := range common.Args {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		proof := analysis.proveCarriedValueWithin(argument, budget)
		if proof.State != proofs.EvidenceDisproven {
			return proof
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

// A struct literal wrapping a type-asserted response body and passed to a
// function value may carry the resource; kandev's SPDY handoff is such a case:
// https://github.com/kdlbs/kandev/blob/17da0aafe33df01828e21fc79cc9dd156dc088dc/apps/backend/internal/agent/kubernetes/portforward.go#L464-L491
func (analysis *resourceAnalysis) proveCarriedValueWithin(value ssa.Value, budget *proofs.SearchBudget) resourceProof {
	direct := analysis.proveCarriedDirectlyWithin(value, budget)
	if direct.State != proofs.EvidenceDisproven {
		return direct
	}
	within := analysis.proveNestedCarryWithin(value, budget)
	if within.State != proofs.EvidenceDisproven {
		return within
	}
	return analysis.provePossibleWrapperWithin(value, 0, false, budget)
}

func (analysis *resourceAnalysis) proveCarriedDirectlyWithin(value ssa.Value, budget *proofs.SearchBudget) resourceProof {
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
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	return carriedValueProof(same.Proven(), resourceReasonDirectMayCarry, budget)
}

func (analysis *resourceAnalysis) directStorageBudget(budget *proofs.SearchBudget) *proofs.SearchBudget {
	if budget == nil {
		return analysis.budget(proofs.QueryBudget)
	}
	return budget.Within(proofs.QueryBudget)
}

func (analysis *resourceAnalysis) proveNestedCarryWithin(value ssa.Value, budget *proofs.SearchBudget) resourceProof {
	contained := lifecycle.ProveMayContainValueWithin(value, analysis.resource, budget)
	if contained.State == proofs.EvidenceUnknown {
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
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

func carriedValueProof(found bool, reason resourceLifetimeReason, budget *proofs.SearchBudget) resourceProof {
	if resourceFlowExhausted(budget) {
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	if found {
		return resourceProof{State: proofs.EvidenceProven, Reason: reason}
	}
	return resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonUntouched}
}

// Callback carrying shares the ordinary carried-value proof after resolving
// captures. These are possible ownership boundaries, never cleanup guarantees.
// Binding/argument visits share the allowance; alias/graph/type and effect
// internals remain independent. Prior-registration analysis uses the same
// bounded capture proof without a separate Boolean adapter.

// Retaining a callback also retains its captured resource. A known test
// cleanup registration has its own coverage proof; a visible observer that
// neither invokes nor retains the callback is not an ownership boundary.
func (analysis *resourceAnalysis) provePossiblyRetainedCallbackWithin(
	instruction ssa.Instruction, common *ssa.CallCommon, budget *proofs.SearchBudget,
) resourceProof {
	if !budget.Spend() {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	if ssacall.HasLibraryContract(common, ssacall.ContractTestingCleanup) {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	for index, argument := range common.Args {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		closure := analysis.proveCarriedClosureWithin(argument, budget)
		if closure.State == proofs.EvidenceUnknown {
			return closure
		}
		if !closure.Proven() {
			continue
		}
		retained, known := analysis.evidence.ArgumentRetained(instruction, index)
		callee := common.StaticCallee()
		if retained || !known && (callee == nil || len(callee.Blocks) == 0) {
			return carriedValueProof(true, resourceReasonCapturedByPossiblyRetainedCallback, budget)
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

// proveCarriedAggregateArgumentsWithin finds a separate owner argument even
// when another argument directly borrows the resource. A reader plus a variadic
// closer list can return ownership through the list without the reader owning it.
func (analysis *resourceAnalysis) proveCarriedAggregateArgumentsWithin(common *ssa.CallCommon, budget *proofs.SearchBudget) resourceProof {
	for _, argument := range common.Args {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		if heapmodel.MayAlias(argument, analysis.resource) {
			continue
		}
		// A closure carrying the resource is excluded from struct ownership;
		// closure/launch policy already decides its fate. An interrupted capture
		// search cannot justify treating it as a different kind of aggregate.
		closure := analysis.proveCarriedClosureWithin(argument, budget)
		if closure.State == proofs.EvidenceUnknown {
			return closure
		}
		if closure.Proven() {
			continue
		}
		nested := analysis.proveNestedCarryWithin(argument, budget)
		if nested.State != proofs.EvidenceDisproven {
			return nested
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

// Keep the existing single transparent step: recognizing a callback argument
// is narrower than the recursive may-containment query over all its wrappers.
func (analysis *resourceAnalysis) proveCarriedClosureWithin(argument ssa.Value, budget *proofs.SearchBudget) resourceProof {
	if !budget.Spend() {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	value := argument
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface
	if inner, ok := ssaflow.UnwrapTransparentValue(value, forms); ok {
		value = inner
	}
	closure, ok := value.(*ssa.MakeClosure)
	if !ok {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	return analysis.proveClosureCarryWithin(closure, budget)
}

func (analysis *resourceAnalysis) proveClosureCarryWithin(closure *ssa.MakeClosure, budget *proofs.SearchBudget) resourceProof {
	if !budget.Spend() {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	for _, binding := range closure.Bindings {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		if heapmodel.CapturedBindingMatchesWithin(binding, analysis.resource, budget) {
			return carriedValueProof(true, resourceReasonAggregateMayCarry, budget)
		}
		if resourceFlowExhausted(budget) {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		carried := analysis.proveCarriedValueWithin(binding, budget)
		if carried.State != proofs.EvidenceDisproven {
			return carried
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}
