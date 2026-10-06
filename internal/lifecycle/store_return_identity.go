package lifecycle

import (
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
	"golang.org/x/tools/go/ssa"
)

// ReturnsParameterUnchanged reports whether every normal return of function
// hands back parameter itself, under the same static type, at result index.
// Identity is exact storage identity, not derivation: a wrapper, a
// conversion to an interface, or a value chosen between the parameter and
// something else is not the parameter. A body with no reachable normal
// return proves nothing.
func ReturnsParameterUnchanged(function *ssa.Function, parameter ssa.Value, index int) bool {
	return ProveReturnedParameterWithin(function, parameter, index, nil).Proven()
}

// ProveReturnedParameterWithin requires a reachable normal return and exact
// same-type parameter identity at every return. Reachability, coverage and
// storage comparisons share budget. Cutoff and unresolved identity are unknown,
// never evidence that no counterexample exists. A nil budget retains default
// flow policy and the storage engine's own allowance; graph construction and
// type-system internals have independent costs.
func ProveReturnedParameterWithin(function *ssa.Function, parameter ssa.Value, index int, budget *proofs.SearchBudget) proofs.Proof {
	unknown := proofs.Proof{Reason: proofs.EvidenceUnavailable}
	if function == nil || len(function.Blocks) == 0 || parameter == nil || index < 0 {
		return unknown
	}
	reachable := ssapath.ProveNormalReturnWithin(function.Blocks[0], nil, budget)
	if !reachable.Proven() {
		unknown.Reason = reachable.Reason
		return unknown
	}
	storage := heapmodel.NewStorage(budget)
	outcome := ssapath.EvaluateObligationFromEntry(function, ssapath.ObligationFlow{
		Budget: budget, Instruction: ssapath.ExactOrNone(nil),
		Return: func(returned *ssa.Return) ssapath.ObligationAction {
			if !budget.Spend() || index >= len(returned.Results) {
				return ssapath.ObligationUnknown
			}
			result := returned.Results[index]
			if types.Identical(result.Type(), parameter.Type()) && storage.Same(result, parameter).Proven() {
				return ssapath.ObligationExact
			}
			return ssapath.ObligationUnknown
		},
	})
	if budget.Exhausted() || storage.Budget().Exhausted() {
		unknown.Reason = proofs.EvidenceBudgetExhausted
		return unknown
	}
	if outcome != ssapath.ObligationHonored {
		return unknown
	}
	return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceStructuralWalk, Provenance: proofs.EvidenceFromLocalSSA}
}
