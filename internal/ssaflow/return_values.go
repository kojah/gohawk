package ssaflow

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ReturnsOnlyNilOrErrors reports whether a nonempty return contains only
// definitely nil values or values of the builtin error type, including aliases.
// This describes the result shape, not failure, ownership or cleanup coverage.
func ReturnsOnlyNilOrErrors(returned *ssa.Return) bool {
	if returned == nil || len(returned.Results) == 0 {
		return false
	}
	errorType := types.Universe.Lookup("error").Type()
	for _, result := range returned.Results {
		if !DefinitelyNil(result) && !types.Identical(result.Type(), errorType) {
			return false
		}
	}
	return true
}
