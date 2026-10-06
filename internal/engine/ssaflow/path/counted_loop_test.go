package path_test

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
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
		reason ssapath.CountedLoopReason
		used   bool
	}{
		{"fixed", 4, ssapath.LoopCountKnown, false},
		{"used", 4, ssapath.LoopCountKnown, true},
		{"fixed", 2, ssapath.LoopCountOverLimit, false},
		{"dynamic", 4, ssapath.LoopShapeUnknown, false},
		{"decrement", 4, ssapath.LoopShapeUnknown, false},
		{"nonzero", 4, ssapath.LoopShapeUnknown, false},
	} {
		got := ssapath.ProveCountedLoop(pkg.Func(test.name).Blocks[1], test.limit, proofs.NewSearchBudget(proofs.QueryBudget))
		if got.Reason != test.reason || got.CounterUsed != test.used {
			t.Errorf("%s limit=%d: %+v", test.name, test.limit, got)
		}
		if got.Proven() && (got.Count != 3 || got.Body == nil || got.Exit == nil) {
			t.Errorf("missing exact count/body/exit: %+v", got)
		}
	}
	cut := ssapath.ProveCountedLoop(pkg.Func("fixed").Blocks[1], 4, proofs.NewSearchBudget(1))
	if cut.Reason != ssapath.LoopBudgetExhausted {
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
		reason ssapath.CountedLoopReason
		count  int
	}{
		{"branching", ssapath.LoopCountKnown, 2},
		{"skipped", ssapath.LoopCountKnown, 3},
		{"broken", ssapath.LoopShapeUnknown, 0},
		{"returned", ssapath.LoopShapeUnknown, 0},
		{"dynamic", ssapath.LoopShapeUnknown, 0},
	} {
		got := ssapath.ProveCountedRegion(pkg.Func(test.name).Blocks[1], 4, proofs.NewSearchBudget(proofs.QueryBudget))
		if got.Reason != test.reason || got.Count != test.count {
			t.Errorf("%s: %+v", test.name, got)
		}
		if got.Proven() && (got.Body != pkg.Func(test.name).Blocks[2] || got.Exit == nil) {
			t.Errorf("%s: missing body/exit: %+v", test.name, got)
		}
	}
	// The straight-line form must still reject a branching body.
	if got := ssapath.ProveCountedLoop(pkg.Func("branching").Blocks[1], 4, proofs.NewSearchBudget(proofs.QueryBudget)); got.Proven() {
		t.Errorf("ProveCountedLoop accepted a branching body: %+v", got)
	}
}
