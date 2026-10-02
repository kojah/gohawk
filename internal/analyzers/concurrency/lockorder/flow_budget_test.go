package lockorder

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/resultfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestLockWalkCutoffDiscardsReportsAndOrders(t *testing.T) {
	// The read-lock write and order witnesses precede the padded tail.
	// A later cutoff must discard already buffered evidence, not just stop
	// discovering further findings. Every cutoff is followed by a fresh query.
	pkg := lockWalkBudgetPackage(t)
	fixture := newLockWalkFixture(pkg.Func("witness"))
	complete, diagnostics, edges := fixture.run(ssaflow.NewSearchBudget(lockStateWorkBudget))
	if !complete || len(diagnostics) == 0 || edges == 0 {
		t.Fatalf("baseline complete=%v reports=%v edges=%d", complete, diagnostics, edges)
	}
	reachedCompletion := false
	for limit := range ssaflow.SummaryBudget {
		pool := ssaflow.NewSearchBudget(lockStateWorkBudget)
		child := pool.Within(limit)
		ok, reports, orders := fixture.run(child)
		if ok {
			reachedCompletion = true
			break
		}
		if !child.Exhausted() || pool.Exhausted() || len(reports) != 0 || orders != 0 {
			t.Fatalf("cut%d exhausted=%v/%v reports=%v edges=%d", limit, child.Exhausted(), pool.Exhausted(), reports, orders)
		}
		recovered, reports, orders := fixture.run(pool.Within(lockStateWorkBudget / 2))
		if !recovered || len(reports) != len(diagnostics) || orders != edges {
			t.Fatalf("fresh after cut%d complete=%v reports=%v edges=%d", limit, recovered, reports, orders)
		}
	}
	if !reachedCompletion {
		t.Fatal("walk never completed")
	}
}

func TestLockWalkParentCutoffAndStableCleanup(t *testing.T) {
	pkg := lockWalkBudgetPackage(t)
	fixture := newLockWalkFixture(pkg.Func("witness"))
	pool := ssaflow.NewSearchBudget(1)
	child := pool.Within(lockStateWorkBudget)
	if ok, reports, orders := fixture.run(child); ok || !child.PoolExhausted() || len(reports) != 0 || orders != 0 {
		t.Fatalf("parent cutoff complete=%v reports=%v edges=%d", ok, reports, orders)
	}
	safe := newLockWalkFixture(pkg.Func("safe"))
	if ok, reports, orders := safe.run(ssaflow.NewSearchBudget(lockStateWorkBudget)); !ok || len(reports) != 0 || orders != 0 {
		t.Fatalf("stable branch cleanup complete=%v reports=%v edges=%d", ok, reports, orders)
	}
}

type lockWalkFixture struct {
	function    *ssa.Function
	evidence    lifecycle.LocalEvidence
	calleeLocks *calleeLockSearch
	results     *resultfacts.Engine
}

func newLockWalkFixture(function *ssa.Function) *lockWalkFixture {
	return &lockWalkFixture{function: function, calleeLocks: newCalleeLockSearch(), results: resultfacts.NewEngine()}
}

func (fixture *lockWalkFixture) run(budget *ssaflow.SearchBudget) (bool, []analysis.Diagnostic, int) {
	var reports []analysis.Diagnostic
	fn := fixture.function
	pass := &analysis.Pass{
		Fset: fn.Prog.Fset, Pkg: fn.Pkg.Pkg,
		ResultOf: map[*analysis.Analyzer]any{resultfacts.Analyzer: fixture.results},
		Report:   func(d analysis.Diagnostic) { reports = append(reports, d) },
	}
	orders := newLockOrders()
	exclusive := newExclusiveCallers(pass, []*ssa.Function{fn})
	complete := walkLockOrderWithin(pass, fn, orders, fixture.calleeLocks, &fixture.evidence, nil, exclusive, budget)
	return complete, reports, len(orders.edges)
}

func lockWalkBudgetPackage(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "lockbudget", `package lockbudget
 import "sync"
 var first, second sync.Mutex
 var total int
 var shared struct { mu sync.RWMutex; value int }
 func witness(n int) {
 shared.mu.RLock(); shared.value++; shared.mu.RUnlock()
 first.Lock(); second.Lock()
 `+strings.Repeat("n += 1; total = n\n", 12)+`
 second.Unlock(); first.Unlock()
 }
 func safe(flag bool) { if flag {first.Lock()}; if flag {first.Unlock()} }
 `)
}
