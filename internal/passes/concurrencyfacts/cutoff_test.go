package concurrencyfacts

import (
	"go/token"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestCutoffProvenanceSurvivesCompositionAndCaching(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "cutoffs", `package cutoffs
func leaf(p *chan int) { _ = *p }
func helper(p *chan int) { leaf(p) }
func first(p *chan int) { helper(p) }
func second(p *chan int) { helper(p) }
func safe(c chan int) { close(c) }
func loop(c chan int, n int) { for i := 0; i < n; i++ { close(c) } }
func opaque(f func()) { f() }
`)
	engine := NewEngine()
	for _, name := range []string{"first", "second", "first"} {
		got := engine.Root(pkg.Func(name), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		if got.Reason != ReasonLoadUnknown || got.Complete() {
			t.Fatalf("%s: unexpected summary %+v", name, got)
		}
		var count int
		got.ObserveCutoff(func(reason string, at token.Pos, details map[string]string) {
			count++
			if reason != "protocol-cutoff" || !at.IsValid() || details["function"] != "cutoffs.leaf" ||
				details["instruction-kind"] != "*ssa.UnOp" || details["call-depth"] != "2" ||
				details["caller-0"] != "cutoffs.helper" || details["caller-1"] != "cutoffs."+name {
				t.Errorf("%s: unexpected attribution %s %v %+v", name, reason, at, details)
			}
		})
		if count != 1 {
			t.Errorf("%s: got %d cutoff events", name, count)
		}
	}
	for _, test := range []struct{ name, shape, instruction string }{
		{"loop", "unsupported-loop-or-branch", "*ssa.If"},
		{"opaque", "instruction", "*ssa.Call"},
	} {
		got := engine.Root(pkg.Func(test.name), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		got.ObserveCutoff(func(_ string, _ token.Pos, details map[string]string) {
			if details["shape"] != test.shape || details["instruction-kind"] != test.instruction {
				t.Errorf("%s: %+v", test.name, details)
			}
		})
		if got.cutoff == nil {
			t.Errorf("%s lost cutoff", test.name)
		}
	}
	got := engine.Root(pkg.Func("safe"), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	got.ObserveCutoff(func(string, token.Pos, map[string]string) { t.Error("complete summary emitted cutoff") })
	if !got.Complete() || got.cutoff != nil {
		t.Fatalf("successful summary retained failure: %+v", got)
	}
}

func TestCutoffChainBoundAndDisabledObserver(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "bounded", `package bounded
func leaf(p *chan int) { _ = *p }
func root(p *chan int) { leaf(p) }
`)
	engine := NewEngine()
	got := engine.Root(pkg.Func("root"), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	original := got.cutoff
	call := original.calls[0]
	for range maxCutoffCalls + 2 {
		got = engine.instantiatedCutoff(got, call)
	}
	if got.cutoff.depth != maxCutoffCalls || !got.cutoff.truncated || original.depth != 1 || original.truncated {
		t.Fatalf("chain bound or immutability lost: original=%+v got=%+v", original, got.cutoff)
	}
	if allocations := testing.AllocsPerRun(100, func() { got.ObserveCutoff(nil) }); allocations != 0 {
		t.Errorf("disabled observation allocated %g times", allocations)
	}
	got.ObserveCutoff(func(_ string, _ token.Pos, details map[string]string) {
		if details["chain-truncated"] != "true" || !strings.Contains(details["instruction"], "*p") {
			t.Errorf("bad bounded trace: %+v", details)
		}
	})
}
