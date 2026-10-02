package concurrencyfacts

import (
	"go/token"
	"reflect"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

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
			fresh, ok := NewEngine().exportFunction(fn, ssaflow.NewSearchBudget(exportBudget))
			if !ok || !reflect.DeepEqual(fresh, want) {
				t.Fatalf("fresh publication=%+v/%v want%+v", fresh, ok, want)
			}
		})
	}
}

func checkPublicationCutoffs(t *testing.T, fn *ssa.Function, engine *Engine, want Fact) {
	t.Helper()
	if got, ok := engine.exportFunction(fn, ssaflow.NewSearchBudget(0)); ok || !reflect.DeepEqual(got, Fact{Version: factVersion}) {
		t.Fatalf("zero allowance retained%+v/%v", got, ok)
	}
	for limit := 0; limit <= exportBudget; limit++ {
		pool := ssaflow.NewSearchBudget(4 * exportBudget)
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

func TestDeclarationSharesInferenceAndPublicationAllowance(t *testing.T) {
	fn := ssaflowtest.BuildPackage(t, "publication", publicationFixture).Func("Linear")
	engine := NewEngine()
	want, ok := engine.Declaration(fn, nil)
	if !ok || len(want.Effects) != 2 || !reflect.DeepEqual(want.Effects[0].Fields, []int{0}) {
		t.Fatalf("declaration=%+v/%v", want, ok)
	}
	// A cached summary saves inference work, but cannot skip field validation.
	if got, ok := engine.Declaration(fn, ssaflow.NewSearchBudget(1)); ok || len(got.Effects) != 0 {
		t.Fatalf("lookup-only allowance retained%+v/%v", got, ok)
	}
	for limit := 0; limit <= exportBudget; limit++ {
		budget := ssaflow.NewSearchBudget(limit)
		got, ok := engine.Declaration(fn, budget)
		if budget.Exhausted() {
			if ok || len(got.Effects) != 0 || len(got.Workers) != 0 {
				t.Fatalf("cut%d retained%+v", limit, got)
			}
			continue
		}
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("complete declaration=%+v/%v", got, ok)
		}
		return
	}
	t.Fatal("declaration never completed")
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
	budget := ssaflow.NewSearchBudget(1)
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
		budget := ssaflow.NewSearchBudget(limit)
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
 func padded(d *D)int{f:=func(){_=d.n};_=f;n:=0;` + strings.Repeat("n++;", ssaflow.QueryBudget+1) + `d.mu.Lock();d.mu.Unlock();return n}`
	fn := ssaflowtest.BuildPackage(t, "publication", source).Func("padded")
	field := ssaflow.InstructionsOf[*ssa.FieldAddr](fn)[0]
	operation := Operation{Kind: Lock, Resource: Reference{Value: field}}
	pool := ssaflow.NewSearchBudget(10 * exportBudget)
	child := pool.Within(ssaflow.QueryBudget)
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
	second.Conditions = []Condition{{Value: fn.Params[0], Context: make([]token.Pos, ssaflow.QueryBudget+1), Holds: true}}
	paths := []Summary{body, second}
	pool := ssaflow.NewSearchBudget(4 * exportBudget)
	child := pool.Within(ssaflow.QueryBudget)
	if got, ok := exportAlternatives(fn, paths, child); ok || !child.Exhausted() || pool.Exhausted() || !reflect.DeepEqual(got, Fact{Version: factVersion}) {
		t.Fatalf("alternative cutoff retained%+v/%v", got, ok)
	}
	got, ok := exportAlternatives(fn, paths, pool.Within(exportBudget))
	if !ok || len(got.Alternatives) != 2 || len(got.Alternatives[1].Conditions) != 1 || got.Alternatives[1].Conditions[0].Parameter != -1 {
		t.Fatalf("fresh alternatives=%+v/%v", got, ok)
	}
}
