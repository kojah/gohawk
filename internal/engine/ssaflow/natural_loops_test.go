package ssaflow_test

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestBackEdgeDominanceDoesNotRequireExitCoverage(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "latches", `package latches
func mark() {}
func conditional(n int, choose bool) { for i:=0; i<n; i++ { if choose { mark() } } }
func breakPath(n int, early bool) { for i:=0; i<n; i++ { if early { break }; mark() } }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"conditional", false},
		{"breakPath", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			loops, ok := ssaflow.OutermostLoops(function, proofs.NewSearchBudget(proofs.QueryBudget))
			if !ok || len(loops) != 1 {
				t.Fatal("expected one natural loop")
			}
			var marked *ssa.BasicBlock
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
				if call.Common().StaticCallee() == pkg.Func("mark") {
					marked = call.Block()
				}
			}
			if marked == nil {
				t.Fatal("missing marker call")
			}
			loop := loops[0]
			if !loop.DominatesBackEdges(loop.Header) {
				t.Error("header did not dominate its back edges")
			}
			if got := loop.DominatesBackEdges(marked); got != test.want {
				t.Errorf("DominatesBackEdges() = %t, want %t", got, test.want)
			}
		})
	}
}

// A loop is bounded only when a counter rises by one toward a bound fixed
// before the loop. A bound the body can grow, a counter the body resets, any
// other step, and a map or Boolean-driven loop are not proven to end.
func TestBoundedLoop(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "loops", `package loops
func slice(xs []int) (n int) { for _, x := range xs { n += x }; return }
func integer(k int) (n int) { for i := range k { n += i }; return }
func counted(k int) (n int) {
	for i := 0; i < k; i++ {
		if i == 3 { break }
		n++
	}
	return
}
func nested(xs [][]int) (n int) { for _, row := range xs { for _, x := range row { n += x } }; return }
func lengths(rows [][]int) (n int) { for _, row := range rows { for i := 0; i < len(row); i++ { n += row[i] } }; return }
func queued(ch chan int) (n int) { for i := 0; i < len(ch); i++ { n++ }; return }
func growing(xs []int) []int { for i := 0; i < len(xs); i++ { xs = append(xs, i) }; return xs }
func reset(k int) (n int) { for i := 0; i < k; i++ { n++; if n > 5 { i = 0 } }; return }
func stepTwo(k int) (n int) { for i := 0; i < k; i += 2 { n++ }; return }
func down(k int) (n int) { for i := k; i > 0; i-- { n++ }; return }
func flag(done *bool) (n int) { for !*done { n++ }; return }
func mapped(m map[string]int) (n int) { for _, v := range m { n += v }; return }
func unbounded(xs [][]int) (n int) { for _, row := range xs { for !done(row) { n++ } }; return }
func done([]int) bool { return true }
`)
	for name, want := range map[string]bool{
		"slice": true, "integer": true, "counted": true, "nested": true, "lengths": true, "queued": false,
		"growing": false, "reset": false, "stepTwo": false, "down": false, "flag": false, "mapped": false, "unbounded": false,
	} {
		loops, ok := ssaflow.OutermostLoops(pkg.Func(name), proofs.NewSearchBudget(1000))
		if !ok || len(loops) != 1 {
			t.Errorf("%s: loops = %v, %t, want one outermost loop", name, loops, ok)
			continue
		}
		if got := ssaflow.BoundedLoop(loops[0], proofs.NewSearchBudget(1000)); got != want {
			t.Errorf("%s: BoundedLoop = %t, want %t", name, got, want)
		}
	}
}
