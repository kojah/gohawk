package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

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
	if ssaflow.HasLibraryContract(common, ssaflow.ContractTestingCleanup) {
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
