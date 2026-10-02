package lockorder

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCalleeConstantAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "constantlocks", `package constantlocks
 import "sync"
 var mu sync.Mutex
 func leaf(yes bool){if yes{mu.Lock();mu.Unlock()}}
 func nested(yes bool){leaf(yes)}
 func caller(){nested(false);nested(true)}
 `)
	fn := pkg.Func("nested")
	for _, test := range []struct {
		outcome ssaflow.Outcome
		count   int
	}{{ssaflow.OutcomeFalse, 0}, {ssaflow.OutcomeTrue, 1}} {
		constants := ssaflow.FixedValues{fn.Params[0]: test.outcome}
		completed := false
		for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
			search := newCalleeLockSearch()
			pool := ssaflow.NewSearchBudget(10 * ssaflow.SummaryBudget)
			child := pool.Within(limit)
			context := &constantContext{budget: child, visiting: map[string]bool{}}
			got, complete := search.locksUnder(fn, constants, context)
			if child.Exhausted() || limit == 0 {
				if complete || len(got.acquires) != 0 || pool.Exhausted() {
					t.Fatalf("cut%d=%+v/%v", limit, got, complete)
				}
				fresh := &constantContext{budget: pool.Within(ssaflow.SummaryBudget), visiting: map[string]bool{}}
				recovered, ok := search.locksUnder(fn, constants, fresh)
				if !ok || len(recovered.acquires) != test.count {
					t.Fatalf("fresh=%+v/%v", recovered, ok)
				}
				continue
			}
			if !complete || len(got.acquires) != test.count {
				t.Fatalf("complete=%+v/%v want%d", got, complete, test.count)
			}
			completed = true
			break
		}
		if !completed {
			t.Fatal("constant query never completed")
		}
	}
	calls := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("caller"))
	if len(calls) != 2 {
		t.Fatalf("root calls=%d", len(calls))
	}
	for index, call := range calls {
		got := newCalleeLockSearch().locksAt(call)
		if len(got.acquires) != index {
			t.Fatalf("root call%d acquired%+v", index, got)
		}
	}
}

func TestCalleeNestedBindingCutoffBeforePruning(t *testing.T) {
	fn := constantPruningPackage(t).Func("root")
	constants := ssaflow.FixedValues{fn.Params[0]: ssaflow.OutcomeNil}
	pool := ssaflow.NewSearchBudget(10 * ssaflow.SummaryBudget)
	child := pool.Within(ssaflow.QueryBudget)
	search := newCalleeLockSearch()
	context := &constantContext{budget: child, visiting: map[string]bool{}}
	got, complete := search.locksUnder(fn, constants, context)
	if complete || len(got.acquires) != 0 || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("cut nested binding=%+v/%v", got, complete)
	}
	fresh := &constantContext{budget: pool.Within(2 * ssaflow.SummaryBudget), visiting: map[string]bool{}}
	if got, complete := search.locksUnder(fn, constants, fresh); !complete || len(got.acquires) != 0 {
		t.Fatalf("fresh pruning=%+v/%v", got, complete)
	}
}

func TestCalleeRootBindingCutoffDoesNotReviveOrders(t *testing.T) {
	pkg := constantPruningPackage(t)
	call := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("call"))[0]
	search := newCalleeLockSearch()
	if ordinary := search.locks(pkg.Func("leaf")); len(ordinary.acquires) != 1 {
		t.Fatalf("ordinary control=%+v", ordinary)
	}
	pool := ssaflow.NewSearchBudget(10 * ssaflow.SummaryBudget)
	child := pool.Within(ssaflow.QueryBudget)
	if got := search.locksAtWithin(call, child); len(got.acquires) != 0 || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("cut revived orders=%+v", got)
	}
	fresh := pool.Within(2 * ssaflow.SummaryBudget)
	if got := search.locksAtWithin(call, fresh); len(got.acquires) != 0 || fresh.Exhausted() {
		t.Fatalf("fresh pruning=%+v", got)
	}
}

func constantPruningPackage(t *testing.T) *ssa.Package {
	t.Helper()
	source := `package prunedlocks
 import "sync"
 var mu sync.Mutex
 func leaf(yes bool,p *int){if yes{mu.Lock();` + strings.Repeat("println(p);", ssaflow.QueryBudget+1) + `if p==nil{println("nil")};mu.Unlock()}}
 func root(p *int){leaf(false,p)}
 func call(){leaf(false,nil)}
 `
	return ssaflowtest.BuildPackage(t, "prunedlocks", source)
}
