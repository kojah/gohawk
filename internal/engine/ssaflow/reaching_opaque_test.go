package ssaflow

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestOpaquePhiFoldsPreserveIdentityBoundary(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "opaque", `package opaque
func merge(first, second int, choose bool) int64 {
    value := first
    if choose { value = second }
    return int64(value)
}
`)
	function := pkg.Func("merge")
	phis := InstructionsOf[*ssa.Phi](function)
	if len(phis) != 1 {
		t.Fatal("expected one merge")
	}
	result := InstructionsOf[*ssa.Return](function)[0].Results[0]
	first := function.Params[0]
	if !NewReachingWalk(TransparentConvert).Any(result, func(_ ReachingWalk, value ssa.Value) bool { return value == first }) {
		t.Fatal("default fold lost a reaching alternative")
	}
	for _, test := range []struct {
		name string
		fold func(ReachingWalk, ssa.Value) bool
	}{
		{"any", func(walk ReachingWalk, target ssa.Value) bool {
			return walk.Any(result, func(_ ReachingWalk, value ssa.Value) bool { return value == target })
		}},
		{"every", func(walk ReachingWalk, target ssa.Value) bool {
			return walk.EveryOf([]ssa.Value{result}, func(_ ReachingWalk, value ssa.Value) bool { return value == target })
		}},
		{"resolve", func(walk ReachingWalk, target ssa.Value) bool {
			_, ok := ResolveReachingValue(walk, result,
				func(_ ReachingWalk, value ssa.Value) (ssa.Value, bool) { return value, value == target },
				func(value ssa.Value) ssa.Value { return value })
			return ok
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			walk := func() ReachingWalk { return NewReachingWalk(TransparentConvert).OpaquePhis() }
			if test.fold(walk(), first) {
				t.Fatal("opaque merge borrowed identity from an alternative")
			}
			if !test.fold(walk(), phis[0]) {
				t.Fatal("opaque merge was not available to the leaf predicate")
			}
			if test.fold(walk().Within(proofs.NewSearchBudget(0)), phis[0]) {
				t.Fatal("exhausted fold supplied identity evidence")
			}
		})
	}
}
