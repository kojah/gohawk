package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestCountedLoopControlEvidence(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "counts", `package counts
func fixed(f func()) { for i := 0; i < 3; i++ { f() } }
func used(f func(int)) { for i := 0; i < 3; i++ { f(i) } }
func dynamic(f func(), n int) { for i := 0; i < n; i++ { f() } }
func decrement(f func()) { for i := 0; i < 3; i-- { f() } }
func nonzero(f func()) { for i := 1; i < 3; i++ { f() } }
`)
	for _, test := range []struct {
		name   string
		limit  int
		reason ssaflow.CountedLoopReason
		used   bool
	}{
		{"fixed", 4, ssaflow.LoopCountKnown, false},
		{"used", 4, ssaflow.LoopCountKnown, true},
		{"fixed", 2, ssaflow.LoopCountOverLimit, false},
		{"dynamic", 4, ssaflow.LoopShapeUnknown, false},
		{"decrement", 4, ssaflow.LoopShapeUnknown, false},
		{"nonzero", 4, ssaflow.LoopShapeUnknown, false},
	} {
		got := ssaflow.ProveCountedLoop(pkg.Func(test.name).Blocks[1], test.limit, ssaflow.NewSearchBudget(ssaflow.QueryBudget))
		if got.Reason != test.reason || got.CounterUsed != test.used {
			t.Errorf("%s limit=%d: %+v", test.name, test.limit, got)
		}
		if got.Proven() && (got.Count != 3 || got.Body == nil || got.Exit == nil) {
			t.Errorf("missing exact count/body/exit: %+v", got)
		}
	}
	cut := ssaflow.ProveCountedLoop(pkg.Func("fixed").Blocks[1], 4, ssaflow.NewSearchBudget(1))
	if cut.Reason != ssaflow.LoopBudgetExhausted {
		t.Errorf("budget cut = %+v", cut)
	}
}

func TestCountedRegionControlEvidence(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "regions", `package regions
func branching(a, b chan int) {
	for i := 0; i < 2; i++ {
		select {
		case <-a:
		case <-b:
		}
	}
}
func skipped(f func() bool) { for i := 0; i < 3; i++ { if f() { continue }; f() } }
func broken(f func() bool) { for i := 0; i < 3; i++ { if f() { break } } }
func returned(f func() bool) { for i := 0; i < 3; i++ { if f() { return } } }
func dynamic(f func(), n int) { for i := 0; i < n; i++ { f() } }
`)
	for _, test := range []struct {
		name   string
		reason ssaflow.CountedLoopReason
		count  int
	}{
		{"branching", ssaflow.LoopCountKnown, 2},
		{"skipped", ssaflow.LoopCountKnown, 3},
		{"broken", ssaflow.LoopShapeUnknown, 0},
		{"returned", ssaflow.LoopShapeUnknown, 0},
		{"dynamic", ssaflow.LoopShapeUnknown, 0},
	} {
		got := ssaflow.ProveCountedRegion(pkg.Func(test.name).Blocks[1], 4, ssaflow.NewSearchBudget(ssaflow.QueryBudget))
		if got.Reason != test.reason || got.Count != test.count {
			t.Errorf("%s: %+v", test.name, got)
		}
		if got.Proven() && (got.Body != pkg.Func(test.name).Blocks[2] || got.Exit == nil) {
			t.Errorf("%s: missing body/exit: %+v", test.name, got)
		}
	}
	// The straight-line form must still reject a branching body.
	if got := ssaflow.ProveCountedLoop(pkg.Func("branching").Blocks[1], 4, ssaflow.NewSearchBudget(ssaflow.QueryBudget)); got.Proven() {
		t.Errorf("ProveCountedLoop accepted a branching body: %+v", got)
	}
}
