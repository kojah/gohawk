package concurrencyfacts

import (
	"go/token"
	"reflect"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
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
		got := engine.Root(pkg.Func(name), proofs.NewSearchBudget(proofs.SummaryBudget))
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
		got := engine.Root(pkg.Func(test.name), proofs.NewSearchBudget(proofs.SummaryBudget))
		got.ObserveCutoff(func(_ string, _ token.Pos, details map[string]string) {
			if details["shape"] != test.shape || details["instruction-kind"] != test.instruction {
				t.Errorf("%s: %+v", test.name, details)
			}
		})
		if got.cutoff == nil {
			t.Errorf("%s lost cutoff", test.name)
		}
	}
	got := engine.Root(pkg.Func("safe"), proofs.NewSearchBudget(proofs.SummaryBudget))
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
	got := engine.Root(pkg.Func("root"), proofs.NewSearchBudget(proofs.SummaryBudget))
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

const publicationFixture = `package publication
import ("sync"; "context")
type Owner struct { mu sync.RWMutex }
func Linear(o *Owner) { o.mu.Lock(); o.mu.Unlock() }
func Worker(o *Owner) { go func(){o.mu.RLock(); o.mu.RUnlock()}() }
func Branch(o *Owner, take bool) bool { if take {o.mu.Lock(); o.mu.Unlock(); return true}; return false }
func Empty() {}
func Cancel(cancel context.CancelFunc) { cancel() }
func Await(ctx context.Context) { <-ctx.Done() }
`

func TestPublicationCutoffDiscardsEveryEffect(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "publication", publicationFixture)
	for _, name := range []string{"Linear", "Worker", "Branch", "Empty", "Cancel", "Await"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			engine := NewEngine()
			want, ok := engine.exportFunction(fn, nil)
			if !ok {
				t.Fatal("unbounded publication unavailable")
			}
			checkPublicationCutoffs(t, fn, engine, want)
			checkColdPublicationCutoffs(t, fn, want)
			fresh, ok := NewEngine().exportFunction(fn, proofs.NewSearchBudget(exportBudget))
			if !ok || !reflect.DeepEqual(fresh, want) {
				t.Fatalf("fresh publication=%+v/%v want%+v", fresh, ok, want)
			}
		})
	}
}

func checkPublicationCutoffs(t *testing.T, fn *ssa.Function, engine *Engine, want Fact) {
	t.Helper()
	if got, ok := engine.exportFunction(fn, proofs.NewSearchBudget(0)); ok || !reflect.DeepEqual(got, Fact{Version: factVersion}) {
		t.Fatalf("zero allowance retained%+v/%v", got, ok)
	}
	for limit := 0; limit <= exportBudget; limit++ {
		pool := proofs.NewSearchBudget(4 * exportBudget)
		child := pool.Within(limit)
		got, ok := engine.exportFunction(fn, child)
		if child.Exhausted() {
			if ok || !reflect.DeepEqual(got, Fact{Version: factVersion}) {
				t.Fatalf("cut%d retained%+v/%v", limit, got, ok)
			}
			if pool.Exhausted() {
				t.Fatal("child cutoff exhausted parent")
			}
			continue
		}
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("complete%d=%+v/%v want%+v", limit, got, ok, want)
		}
		return
	}
	t.Fatal("publication never completed")
}

func TestInterruptedProjectionCannotUsePreviouslyInferredRoot(t *testing.T) {
	fn := ssaflowtest.BuildPackage(t, "publication", publicationFixture).Func("Linear")
	summary := NewEngine().Function(fn, nil)
	if len(summary.Operations) != 2 {
		t.Fatalf("summary=%+v", summary)
	}
	operation := summary.Operations[0]
	path, ok := embeddedPathWithin(operation.Resource.Value, nil)
	if !ok {
		t.Fatal("no embedded path")
	}
	operation.Resource.Projection = path
	budget := proofs.NewSearchBudget(1)
	if effect, ok := exportEffect(fn, operation, budget); ok || !budget.Exhausted() {
		t.Fatalf("interrupted path retained%+v/%v", effect, ok)
	}
	if effect, ok := exportEffect(fn, operation, nil); !ok || !reflect.DeepEqual(effect.Fields, []int{0}) {
		t.Fatalf("fresh effect=%+v/%v", effect, ok)
	}
}

func checkColdPublicationCutoffs(t *testing.T, fn *ssa.Function, want Fact) {
	t.Helper()
	for limit := 0; limit <= exportBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		got, ok := NewEngine().exportFunction(fn, budget)
		if budget.Exhausted() {
			if ok || !reflect.DeepEqual(got, Fact{Version: factVersion}) {
				t.Fatalf("cold cut%d retained%+v/%v", limit, got, ok)
			}
			continue
		}
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("cold complete%d=%+v/%v", limit, got, ok)
		}
		return
	}
	t.Fatal("cold publication never completed")
}

func TestPublicationFieldSearchSharesAllowance(t *testing.T) {
	source := `package publication
 import "sync"
 type D struct{mu sync.Mutex;n int}
 func padded(d *D)int{f:=func(){_=d.n};_=f;n:=0;` + strings.Repeat("n++;", proofs.QueryBudget+1) + `d.mu.Lock();d.mu.Unlock();return n}`
	fn := ssaflowtest.BuildPackage(t, "publication", source).Func("padded")
	field := ssaflow.InstructionsOf[*ssa.FieldAddr](fn)[0]
	operation := Operation{Kind: Lock, Resource: Reference{Value: field}}
	pool := proofs.NewSearchBudget(10 * exportBudget)
	child := pool.Within(proofs.QueryBudget)
	if effect, ok := exportEffect(fn, operation, child); ok || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("cut retained%+v/%v", effect, ok)
	}
	effect, ok := exportEffect(fn, operation, pool.Within(2*exportBudget))
	if !ok || effect.Parameter != 0 || !reflect.DeepEqual(effect.Fields, []int{0}) {
		t.Fatalf("fresh effect=%+v/%v", effect, ok)
	}
}

func TestAlternativeCutoffDiscardsEarlierPaths(t *testing.T) {
	fn := ssaflowtest.BuildPackage(t, "publication", publicationFixture).Func("Linear")
	body := NewEngine().Function(fn, nil)
	second := body
	second.Conditions = []Condition{{Value: fn.Params[0], Context: make([]token.Pos, proofs.QueryBudget+1), Holds: true}}
	paths := []Summary{body, second}
	pool := proofs.NewSearchBudget(4 * exportBudget)
	child := pool.Within(proofs.QueryBudget)
	if got, ok := exportAlternatives(fn, paths, child); ok || !child.Exhausted() || pool.Exhausted() || !reflect.DeepEqual(got, Fact{Version: factVersion}) {
		t.Fatalf("alternative cutoff retained%+v/%v", got, ok)
	}
	got, ok := exportAlternatives(fn, paths, pool.Within(exportBudget))
	if !ok || len(got.Alternatives) != 2 || len(got.Alternatives[1].Conditions) != 1 || got.Alternatives[1].Conditions[0].Parameter != -1 {
		t.Fatalf("fresh alternatives=%+v/%v", got, ok)
	}
}
