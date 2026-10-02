package lifecycle

import (
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
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
func ProveReturnedParameterWithin(function *ssa.Function, parameter ssa.Value, index int, budget *ssaflow.SearchBudget) ssaflow.Proof {
	unknown := ssaflow.Proof{Reason: ssaflow.EvidenceUnavailable}
	if function == nil || len(function.Blocks) == 0 || parameter == nil || index < 0 {
		return unknown
	}
	reachable := ssaflow.ProveNormalReturnWithin(function.Blocks[0], nil, budget)
	if !reachable.Proven() {
		unknown.Reason = reachable.Reason
		return unknown
	}
	storage := heapmodel.NewStorage(budget)
	outcome := ssaflow.EvaluateObligationFromEntry(function, ssaflow.ObligationFlow{
		Budget: budget, Instruction: ssaflow.ExactOrNone(nil),
		Return: func(returned *ssa.Return) ssaflow.ObligationAction {
			if !budget.Spend() || index >= len(returned.Results) {
				return ssaflow.ObligationUnknown
			}
			result := returned.Results[index]
			if types.Identical(result.Type(), parameter.Type()) && storage.Same(result, parameter).Proven() {
				return ssaflow.ObligationExact
			}
			return ssaflow.ObligationUnknown
		},
	})
	if budget.Exhausted() || storage.Budget().Exhausted() {
		unknown.Reason = ssaflow.EvidenceBudgetExhausted
		return unknown
	}
	if outcome != ssaflow.ObligationHonored {
		return unknown
	}
	return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceStructuralWalk, Provenance: ssaflow.EvidenceFromLocalSSA}
}
