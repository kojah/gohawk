package lockorder

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/concurrencyfacts"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestLockSetupInventoryAndOwnership(t *testing.T) {
	pkg := lockSetupPackage(t)
	for _, test := range []struct {
		name                      string
		direct, defers, summaries int
		acquisition, borrowed     bool
	}{
		{"borrow", 3, 1, 0, true, true},
		{"fresh", 2, 1, 0, true, false},
		{"via", 0, 0, 1, true, false},
		{"empty", 0, 0, 0, false, false},
		{"builtin", 2, 0, 0, true, false},
	} {
		fn := pkg.Func(test.name)
		pass := lockSetupPass(fn, concurrencyfacts.NewEngine())
		proof := buildLockSetup(pass, fn, proofs.NewSearchBudget(lockStateWorkBudget))
		if !proof.Proven() || proof.setup == nil {
			t.Fatalf("%s: %+v", test.name, proof)
		}
		setup := proof.setup
		countsMatch := len(setup.direct) == test.direct && len(setup.defers) == test.defers && len(setup.summaries) == test.summaries
		if !countsMatch || setup.hasAcquisition != test.acquisition {
			t.Fatalf("%s inventory: %+v", test.name, setup)
		}
		if test.acquisition && setup.callerOwned[lockIdentityOf(fn.Params[0])] != test.borrowed {
			t.Fatalf("%s caller ownership: %+v", test.name, setup.callerOwned)
		}
		if test.name == "via" {
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			if len(setup.summaries[call]) != 2 {
				t.Fatalf("helper effects: %+v", setup.summaries[call])
			}
		}
	}
}

func TestLockSetupCutoffDiscardsInventory(t *testing.T) {
	pkg := lockSetupPackage(t)
	for _, name := range []string{"borrow", "via", "empty"} {
		fn := pkg.Func(name)
		finished := false
		for limit := range proofs.SummaryBudget {
			engine := concurrencyfacts.NewEngine()
			pass := lockSetupPass(fn, engine)
			pool := proofs.NewSearchBudget(lockStateWorkBudget)
			child := pool.Within(limit)
			proof := buildLockSetup(pass, fn, child)
			if proof.Proven() {
				if child.Exhausted() || proof.setup == nil || proof.setup.hasAcquisition != (name != "empty") {
					t.Fatalf("%s accepted interrupted inventory at%d: %+v", name, limit, proof)
				}
				finished = true
				break
			}
			if proof.setup != nil || proof.Reason != proofs.EvidenceBudgetExhausted || !child.Exhausted() || pool.Exhausted() {
				t.Fatalf("%s cut%d: %+v exhausted=%v/%v", name, limit, proof, child.Exhausted(), pool.Exhausted())
			}
			fresh := buildLockSetup(pass, fn, pool.Within(proofs.SummaryBudget))
			if !fresh.Proven() || fresh.setup.hasAcquisition != (name != "empty") {
				t.Fatalf("%s fresh%d: %+v", name, limit, fresh)
			}
			warm := buildLockSetup(pass, fn, pool.Within(0))
			if warm.Proven() || warm.setup != nil {
				t.Fatalf("%s warm zero: %+v", name, warm)
			}
		}
		if !finished {
			t.Fatalf("%s setup never completed", name)
		}
	}
}

func lockSetupPass(fn *ssa.Function, engine *concurrencyfacts.Engine) *analysis.Pass {
	return &analysis.Pass{Fset: fn.Prog.Fset, Pkg: fn.Pkg.Pkg, ResultOf: map[*analysis.Analyzer]any{concurrencyfacts.Analyzer: engine}}
}

func lockSetupPackage(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "locksetup", `package locksetup
 import "sync"
 func borrow(mu *sync.Mutex){mu.Unlock();mu.Lock();defer mu.Unlock()}
 func fresh(mu *sync.Mutex){mu.Lock();defer mu.Unlock()}
 func helper(mu *sync.Mutex){mu.Lock();mu.Unlock()}
 func via(mu *sync.Mutex){helper(mu)}
 func empty(){}
 func builtin(mu *sync.Mutex,a,b []byte){mu.Lock();copy(a,b);mu.Unlock()}
 type wrapper struct {sync.RWMutex;value int}
 func (w *wrapper) Enter()
 func opaque(w *wrapper){w.Enter();defer w.RWMutex.Unlock();w.RWMutex.RLock();w.value++;w.RWMutex.RUnlock()}
 `)
}

func TestLockSetupDeferredWriterWitness(t *testing.T) {
	fn := lockSetupPackage(t).Func("opaque")
	pass := lockSetupPass(fn, concurrencyfacts.NewEngine())
	proof := buildLockSetup(pass, fn, proofs.NewSearchBudget(lockStateWorkBudget))
	if !proof.Proven() || len(proof.setup.possibleWriters) != 1 || len(proof.setup.defers) != 1 ||
		proof.setup.possibleWriters[0] != proof.setup.defers[0] {
		t.Fatalf("opaque wrapper witness: %+v", proof)
	}
}

func TestLockSetupNestedSummaryAllowance(t *testing.T) {
	for _, count := range []int{150, 2500} {
		pkg := ssaflowtest.BuildPackage(t, "locksetupnested", `package locksetupnested
  import "sync"
  func clean(mu *sync.Mutex){mu.Lock();mu.Unlock()}
  func slow(mu *sync.Mutex,n int)int {`+strings.Repeat("n+=n\n", count)+`mu.Lock();mu.Unlock();return n}
  func root(mu *sync.Mutex,n int){clean(mu);slow(mu,n)}
  `)
		fn := pkg.Func("root")
		pass := lockSetupPass(fn, concurrencyfacts.NewEngine())
		limit := 100
		if count > proofs.SummaryBudget {
			limit = lockStateWorkBudget
		}
		budget := proofs.NewSearchBudget(limit)
		proof := buildLockSetup(pass, fn, budget)
		if proof.Proven() || proof.setup != nil || proof.Reason != proofs.EvidenceBudgetExhausted || budget.Exhausted() != (count < proofs.SummaryBudget) {
			t.Fatalf("padded%d setup: %+v exhausted=%v", count, proof, budget.Exhausted())
		}
		if count < proofs.SummaryBudget {
			fresh := buildLockSetup(pass, fn, proofs.NewSearchBudget(lockStateWorkBudget))
			if !fresh.Proven() || !fresh.setup.hasAcquisition || len(fresh.setup.summaries) != 2 {
				t.Fatalf("fresh padded setup: %+v", fresh)
			}
		}
	}
}

func TestLockSetupBindingCutoff(t *testing.T) {
	fn := lockSetupPackage(t).Func("via")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	summary := concurrencyfacts.NewEngine().AtCall(call, proofs.NewSearchBudget(proofs.SummaryBudget))
	if !summary.Complete() || len(summary.Operations) != 2 {
		t.Fatalf("helper summary: %+v", summary)
	}
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	child := pool.Within(1)
	if effects, complete := bindMutexEffects(call, summary.Operations, child); complete || len(effects) != 0 || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("partial effect binding: %+v/%v", effects, complete)
	}
	if effects, complete := bindMutexEffects(call, summary.Operations, pool.Within(proofs.SummaryBudget)); !complete || len(effects) != 2 {
		t.Fatalf("fresh effect binding: %+v/%v", effects, complete)
	}
}
