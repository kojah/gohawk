package ssaflow

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestReachingRevisitObservationPreservesResults(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "revisits", `package revisits
 func value(x int)int{return x}
 func siblings(flag bool,x int)int64{var r int64;if flag{r=int64(x)}else{r=int64(x)};return r}
 `)
	value := pkg.Func("value").Params[0]
	leaf := func(_ ReachingWalk, candidate ssa.Value) bool { return candidate == value }
	for _, mode := range []string{"any", "every", "mark", "resolve"} {
		observed := 0
		walk := NewReachingWalk(TransparentNone).OnRevisit(func() { observed++ })
		ask := func() bool {
			switch mode {
			case "any":
				return walk.Any(value, leaf)
			case "every":
				return walk.Every(value, leaf)
			case "mark":
				return walk.Mark(value)
			default:
				_, ok := ResolveReachingValue(walk, value,
					func(_ ReachingWalk, candidate ssa.Value) (ssa.Value, bool) { return candidate, candidate == value },
					func(candidate ssa.Value) string { return candidate.Name() })
				return ok
			}
		}
		if !ask() || observed != 0 || ask() || observed != 1 {
			t.Fatalf("%s guard/observation changed: revisits=%d", mode, observed)
		}
	}
	fn := pkg.Func("siblings")
	result := InstructionsOf[*ssa.Return](fn)[0].Results[0]
	observed := 0
	walk := NewReachingWalk(TransparentConvert).OnRevisit(func() { observed++ })
	if !walk.Every(result, func(_ ReachingWalk, value ssa.Value) bool { return value == fn.Params[1] }) || observed != 0 {
		t.Fatal("independent sibling origins were conflated")
	}
	walk = NewReachingWalk(TransparentConvert).OnRevisit(func() { observed++ })
	walk.Mark(fn.Params[1])
	if walk.Every(result, func(_ ReachingWalk, value ssa.Value) bool { return value == fn.Params[1] }) || observed != 1 {
		t.Fatal("sibling fold lost its revisit observer")
	}
}
