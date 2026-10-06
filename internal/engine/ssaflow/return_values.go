package ssaflow

import (
	"go/types"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"golang.org/x/tools/go/ssa"
)

// ReturnsOnlyNilOrErrors reports whether a nonempty return contains only
// definitely nil values or values of the builtin error type, including aliases.
// This describes the result shape, not failure, ownership or cleanup coverage.
func ReturnsOnlyNilOrErrors(returned *ssa.Return) bool {
	return ReturnsOnlyNilOrErrorsWithin(returned, nil)
}

// ReturnsOnlyNilOrErrorsWithin shares result and nilness visits with budget.
// Cutoff cannot supply the unsuccessful-construction exception.
func ReturnsOnlyNilOrErrorsWithin(returned *ssa.Return, budget *proofs.SearchBudget) bool {
	if returned == nil || len(returned.Results) == 0 {
		return false
	}
	errorType := types.Universe.Lookup("error").Type()
	for _, result := range returned.Results {
		if !budget.Spend() {
			return false
		}
		if !DefinitelyNilWithin(result, budget) && !types.Identical(result.Type(), errorType) {
			return false
		}
	}
	return !budget.Exhausted() && !budget.PoolExhausted()
}
