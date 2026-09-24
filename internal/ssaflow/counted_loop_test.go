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
