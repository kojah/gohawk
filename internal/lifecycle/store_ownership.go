package lifecycle

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func closureCallsValue(closure *ssa.MakeClosure, target ssa.Value) bool {
	return closureCallsCapturedValue(closure, func(binding ssa.Value) bool {
		return heapmodel.CapturedBindingMatches(binding, target)
	})
}

func closureCallsCapturedValue(closure *ssa.MakeClosure, owns func(ssa.Value) bool) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	bindings := ssaflow.ClosureBindingPairs(function, closure)
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			if nested, ok := candidate.(*ssa.MakeClosure); ok && closureCallsCapturedValue(nested, func(binding ssa.Value) bool {
				for _, pair := range bindings {
					if heapmodel.CapturedBindingMatches(binding, pair.Free) && owns(pair.Binding) {
						return true
					}
				}
				return false
			}) {
				return true
			}
			common := ssaflow.InstructionCall(candidate)
			if common == nil {
				continue
			}
			for _, pair := range bindings {
				if heapmodel.ValueDerivesFrom(common.Value, pair.Free) && owns(pair.Binding) {
					return true
				}
			}
		}
	}
	return false
}

// MayContainValue reports whether owner may be an aggregate or closure that
// transitively contains value. Possible containment only: it can hide a
// diagnostic behind an opaque owner, never prove that the owner settles it.
func MayContainValue(owner, value ssa.Value) bool {
	return ProveMayContainValueWithin(owner, value, nil).Proven()
}

// ProveMayContainValueWithin shares value, aggregate and capture traversal with
// budget. Graph construction, graph-query and type internals remain separate.
// Cutoff is unknown; a negative means no modeled containment, not actual absence.
func ProveMayContainValueWithin(owner, value ssa.Value, budget *proofs.SearchBudget) proofs.Proof {
	return proveContainmentWithin(owner, value, budget, func() bool { return heapmodel.Contains(owner, value) })
}

func proveContainmentWithin(owner, value ssa.Value, budget *proofs.SearchBudget, graphContains func() bool) proofs.Proof {
	if !budget.Spend() {
		return proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}
	}
	found := false
	if heapmodel.CanHoldReference(owner.Type()) {
		search := newOwnershipSearch(nil)
		search.budget = budget
		found = valueOwnsValueWithin(owner, value, budget) || search.aggregateStoresValue(owner, value)
		// Graph containment adds copies, merges and captured cells; visible
		// constructors remain the recursive search's responsibility.
		if !found && !search.exhausted() && budget.Spend() {
			found = graphContains()
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}
	}
	state, reason := proofs.EvidenceDisproven, proofs.EvidenceNotFound
	if found {
		state, reason = proofs.EvidenceProven, proofs.EvidenceStructuralWalk
	}
	return proofs.Proof{State: state, Reason: reason, Provenance: proofs.EvidenceFromLocalSSA}
}

// ProveMayContainValueAtWithin asks structural may-containment with its graph
// fallback observed at at. Later visible stores remain possible structural
// ownership, never an exact before-call guarantee or cleanup. The graph observes
// a call's argument before the callee's stores. Structural visits share budget;
// graph construction, graph-query and type internals remain independent. Cutoff
// is unknown; a completed negative means no modeled relation, including when
// the graph cannot answer. A nil budget retains the existing default policy.
func ProveMayContainValueAtWithin(owner, value ssa.Value, at ssa.Instruction, budget *proofs.SearchBudget) proofs.Proof {
	return proveContainmentWithin(owner, value, budget, func() bool {
		contained, known := heapmodel.ContainsAt(owner, value, at)
		return known && contained
	})
}

func valueOwnsValue(owner, value ssa.Value) bool { return valueOwnsValueWithin(owner, value, nil) }

func valueOwnsValueWithin(owner, value ssa.Value, budget *proofs.SearchBudget) bool {
	found := false
	ssaflow.WalkStatesWithin([]ssa.Value{owner}, func(owner ssa.Value) ssa.Value { return owner }, func(owner ssa.Value) ([]ssa.Value, bool) {
		if owner == nil {
			return nil, true
		}
		// A possible alias is evidence before wrappers are peeled. Only
		// wrappers and closure captures extend this narrow ownership query;
		// it does not independently fan out phi alternatives or call results.
		if budget.Spend() && heapmodel.MayAlias(owner, value) {
			found = true
			return nil, false
		}
		if inner, ok := ssaflow.UnwrapTransparentValue(
			owner, ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
		); ok {
			return []ssa.Value{inner}, true
		}
		var successors []ssa.Value
		if closure, ok := owner.(*ssa.MakeClosure); ok {
			found = closureBindingsOwnValueWithin(closure, value, budget, func(binding ssa.Value) bool {
				successors = append(successors, binding)
				return false
			})
		}
		return successors, !found
	}, budget)
	return found && !budget.Exhausted() && !budget.PoolExhausted()
}

// Capture identity and cell contents are shared mechanics. The caller chooses
// whether to follow only nested callbacks or also owning aggregates.
func closureBindingsOwnValueWithin(closure *ssa.MakeClosure, value ssa.Value, budget *proofs.SearchBudget, owns func(ssa.Value) bool) bool {
	for _, binding := range closure.Bindings {
		if !budget.Spend() {
			return false
		}
		if heapmodel.CapturedBindingMatchesWithin(binding, value, budget) || owns(ssaflow.CapturedBindingValueWithin(binding, budget)) {
			return !budget.Exhausted() && !budget.PoolExhausted()
		}
	}
	return false
}

// ProvePossibleClosureCaptureWithin reports whether any closure reaching callback
// may transitively contain target. Phi alternatives are transparent; conversions
// and loads remain opaque. A positive capture is not proof of invocation or
// cleanup. Cutoff is unknown, and a negative means no modeled capture.
// Graph construction and graph queries retain their independent bounds.
func ProvePossibleClosureCaptureWithin(callback, target ssa.Value, budget *proofs.SearchBudget) proofs.Proof {
	if budget == nil {
		budget = proofs.NewSearchBudget(proofs.QueryBudget)
	}
	captured := ssaflow.NewReachingWalk(ssaflow.TransparentNone).Within(budget).Any(callback, func(_ ssaflow.ReachingWalk, leaf ssa.Value) bool {
		if _, closure := leaf.(*ssa.MakeClosure); !closure {
			return false
		}
		return ProveMayContainValueWithin(leaf, target, budget).Proven()
	})
	if budget.Exhausted() {
		return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
	}
	if captured {
		return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceCapturedByClosure}
	}
	return proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound}
}
