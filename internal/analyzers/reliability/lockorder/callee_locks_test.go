package lockorder

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestCalleeLockSummaryBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "locks", `package locks
import "sync"
var first, second sync.Mutex
func leaf() { second.Lock(); second.Unlock() }
func root() { first.Lock(); leaf(); first.Unlock() }
`)
	search := newCalleeLockSearch()
	root := pkg.Func("root")
	limited := ssaflow.NewSearchBudget(1)
	if got := search.summaries.Function(root, limited); len(got.acquires) != 0 || !limited.Exhausted() {
		t.Fatalf("budget-shortened summary retained witnesses: %+v, exhausted=%v", got, limited.Exhausted())
	}
	got := search.locks(root)
	if len(got.acquires) != 2 {
		t.Fatalf("fresh budget did not recover both lock classes: %+v", got)
	}
	if len(got.acquires[1].calls) != 1 {
		t.Errorf("nested acquisition lost call-site provenance: %+v", got.acquires[1])
	}
	if cached := search.summaries.Function(root, ssaflow.NewSearchBudget(0)); len(cached.acquires) != 2 {
		t.Errorf("complete acquisition summary was not cached: %+v", cached)
	}
}

func TestRecursiveCalleeLockSummariesAreNotCached(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "locks", `package locks
import "sync"
var first, second sync.Mutex
func a() { b(); first.Lock(); first.Unlock() }
func b() { a(); second.Lock(); second.Unlock() }
`)
	search := newCalleeLockSearch()
	for _, name := range []string{"a", "b", "a"} {
		got := search.locks(pkg.Func(name))
		if len(got.acquires) != 2 {
			t.Errorf("%s reused a path-shortened answer: %+v", name, got)
		}
	}
}
