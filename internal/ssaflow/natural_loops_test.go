package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

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
		loops, ok := ssaflow.OutermostLoops(pkg.Func(name), ssaflow.NewSearchBudget(1000))
		if !ok || len(loops) != 1 {
			t.Errorf("%s: loops = %v, %t, want one outermost loop", name, loops, ok)
			continue
		}
		if got := ssaflow.BoundedLoop(loops[0], ssaflow.NewSearchBudget(1000)); got != want {
			t.Errorf("%s: BoundedLoop = %t, want %t", name, got, want)
		}
	}
}
