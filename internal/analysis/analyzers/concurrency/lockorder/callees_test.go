package lockorder

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
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
	limited := proofs.NewSearchBudget(1)
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
	if cached := search.summaries.Function(root, proofs.NewSearchBudget(0)); len(cached.acquires) != 2 {
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
		outcome ssacall.Outcome
		count   int
	}{{ssacall.OutcomeFalse, 0}, {ssacall.OutcomeTrue, 1}} {
		constants := ssacall.FixedValues{fn.Params[0]: test.outcome}
		completed := false
		for limit := 0; limit <= proofs.SummaryBudget; limit++ {
			search := newCalleeLockSearch()
			pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
			child := pool.Within(limit)
			context := &constantContext{budget: child, visiting: map[string]bool{}}
			got, complete := search.locksUnder(fn, constants, context)
			if child.Exhausted() || limit == 0 {
				if complete || len(got.acquires) != 0 || pool.Exhausted() {
					t.Fatalf("cut%d=%+v/%v", limit, got, complete)
				}
				fresh := &constantContext{budget: pool.Within(proofs.SummaryBudget), visiting: map[string]bool{}}
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
	constants := ssacall.FixedValues{fn.Params[0]: ssacall.OutcomeNil}
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	child := pool.Within(proofs.QueryBudget)
	search := newCalleeLockSearch()
	context := &constantContext{budget: child, visiting: map[string]bool{}}
	got, complete := search.locksUnder(fn, constants, context)
	if complete || len(got.acquires) != 0 || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("cut nested binding=%+v/%v", got, complete)
	}
	fresh := &constantContext{budget: pool.Within(2 * proofs.SummaryBudget), visiting: map[string]bool{}}
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
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	child := pool.Within(proofs.QueryBudget)
	if got := search.locksAtWithin(call, child); len(got.acquires) != 0 || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("cut revived orders=%+v", got)
	}
	fresh := pool.Within(2 * proofs.SummaryBudget)
	if got := search.locksAtWithin(call, fresh); len(got.acquires) != 0 || fresh.Exhausted() {
		t.Fatalf("fresh pruning=%+v", got)
	}
}

func constantPruningPackage(t *testing.T) *ssa.Package {
	t.Helper()
	source := `package prunedlocks
 import "sync"
 var mu sync.Mutex
 func leaf(yes bool,p *int){if yes{mu.Lock();` + strings.Repeat("println(p);", proofs.QueryBudget+1) + `if p==nil{println("nil")};mu.Unlock()}}
 func root(p *int){leaf(false,p)}
 func call(){leaf(false,nil)}
 `
	return ssaflowtest.BuildPackage(t, "prunedlocks", source)
}

func TestLockCallerInventoryScopeAndUses(t *testing.T) {
	pkg := lockCallerPackage(t)
	functions := []*ssa.Function{nil, pkg.Func("caller"), pkg.Func("many")}
	inventory := collectLockCallers(pkg.Func("init"), functions, proofs.NewSearchBudget(callerSetBudget))
	var dump strings.Builder
	for _, name := range []string{"init", "caller"} {
		if _, err := pkg.Func(name).WriteTo(&dump); err != nil {
			t.Fatal(err)
		}
	}
	t.Log(dump.String())
	for _, test := range []struct {
		name    string
		calls   int
		escaped bool
	}{
		{"duringInit", 1, false},
		{"direct", 1, false},
		{"handed", 0, true},
		{"async", 0, true},
		{"deferred", 0, true},
		{"crowded", 32, true},
	} {
		entry := inventory[pkg.Func(test.name)]
		if len(entry.Calls) != test.calls || entry.Escaped != test.escaped {
			t.Errorf("%s conditional callers: %+v", test.name, entry)
		}
	}
	if _, included := inventory[pkg.Func("Exported")]; included {
		t.Fatal("exported function became a private caller contract")
	}
}

func TestLockCallerCutoffDiscardsPrefix(t *testing.T) {
	pkg := lockCallerPackage(t)
	functions := []*ssa.Function{pkg.Func("caller"), pkg.Func("many")}
	complete := collectLockCallers(pkg.Func("init"), functions, nil)
	finished := false
	for limit := range 1000 {
		pool := proofs.NewSearchBudget(callerSetBudget)
		child := pool.Within(limit)
		inventory := collectLockCallers(pkg.Func("init"), functions, child)
		if inventory != nil {
			if child.Exhausted() || len(inventory) != len(complete) {
				t.Fatalf("cutoff %d published incomplete conditional callers", limit)
			}
			finished = true
			break
		}
		if !child.Exhausted() || pool.Exhausted() {
			t.Fatalf("cutoff %d lost child allowance ownership", limit)
		}
		fresh := collectLockCallers(pkg.Func("init"), functions, pool.Within(callerSetBudget))
		if fresh == nil || len(fresh[pkg.Func("crowded")].Calls) != 32 {
			t.Fatalf("cutoff %d contaminated fresh discovery", limit)
		}
	}
	if !finished {
		t.Fatal("package inventory never completed")
	}
}

func lockCallerPackage(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "lockcallers", `package lockcallers
func duringInit()int{return 0}
var value=duringInit()
func direct(){}
func handed(){}
func async(){}
func deferred(){}
func crowded(){}
func Exported(){}
func take(callback func()){}
func caller(dynamic func()){
 direct();take(handed);go async();defer deferred();Exported();dynamic();println(value)
}
func many(){`+strings.Repeat("crowded();", 33)+`}
`)
}
