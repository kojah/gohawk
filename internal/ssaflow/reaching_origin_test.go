package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestReachingOriginWitnessAndCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "origins", `package origins
func merge(flag bool,x,y int) int { v:=x; if flag { v=y }; return v }
`)
	function := pkg.Func("merge")
	value := ssaflow.InstructionsOf[*ssa.Return](function)[0].Results[0]
	if _, ok := value.(*ssa.Phi); !ok {
		t.Fatal("expected compiled SSA phi")
	}
	noLeaf := func(ssaflow.ReachingWalk, ssa.Value) bool { return false }
	origin := func(candidate ssa.Value) bool { return candidate == value }
	fresh := ssaflow.NewSearchBudget(1)
	if !ssaflow.NewReachingWalk(0).Within(fresh).AnyIncludingOrigin(value, origin, noLeaf) || fresh.Exhausted() {
		t.Fatal("direct phi identity must need only its origin visit")
	}
	cutoff := ssaflow.NewSearchBudget(1)
	cutoff.Spend()
	if ssaflow.NewReachingWalk(0).Within(cutoff).AnyIncludingOrigin(value, origin, noLeaf) || !cutoff.Exhausted() {
		t.Fatal("exhausted request must not accept direct identity")
	}
	walk := ssaflow.NewReachingWalk(0)
	walk.Mark(value)
	if !walk.AnyIncludingOrigin(value, origin, noLeaf) {
		t.Fatal("direct identity is independent of circular alternative evidence")
	}
	leaf := func(_ ssaflow.ReachingWalk, candidate ssa.Value) bool { return candidate == function.Params[1] }
	if !ssaflow.NewReachingWalk(0).AnyIncludingOrigin(value, func(ssa.Value) bool { return false }, leaf) {
		t.Fatal("unmatched origin must retain ordinary alternative traversal")
	}
	if ssaflow.NewReachingWalk(0).AnyIncludingOrigin(nil, func(ssa.Value) bool { return true }, noLeaf) {
		t.Fatal("nil cannot supply an origin witness")
	}
}
