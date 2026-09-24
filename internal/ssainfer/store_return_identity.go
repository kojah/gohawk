package ssainfer

import (
	"go/types"

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
	if function == nil || len(function.Blocks) == 0 || !ssaflow.NormalReturnReachableFrom(function.Blocks[0]) {
		return false
	}
	return !ssaflow.UnownedReturnFromEntryAllow(
		function,
		func(ssa.Instruction) bool { return false },
		func(returned *ssa.Return) bool {
			if index < 0 || index >= len(returned.Results) {
				return false
			}
			result := returned.Results[index]
			return types.Identical(result.Type(), parameter.Type()) &&
				NewStorage(nil).Same(result, parameter).Proven()
		},
	)
}
