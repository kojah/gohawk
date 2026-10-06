package lockorder

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/resultfacts"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
)

func TestLockSuccessorResultInferenceSharesAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "lockbranches", `package lockbranches
 var total int
 func allowed()bool {`+strings.Repeat("total++\n", 50)+`return true}
 func root(){if allowed(){total++}}
 `)
	fn := pkg.Func("root")
	state := lockFlowState{block: fn.Blocks[0]}
	pass := &analysis.Pass{Fset: fn.Prog.Fset, Pkg: fn.Pkg.Pkg}
	// Measure ordinary successor setup independently of result inference. The
	// extra allowance below fits that setup but cannot fit the padded callee.
	setup := 0
	for ; setup < proofs.QueryBudget; setup++ {
		budget := proofs.NewSearchBudget(setup)
		got := lockSuccessorStates(pass, state, budget)
		if !budget.Exhausted() && len(got) == 2 {
			break
		}
	}
	if setup == proofs.QueryBudget {
		t.Fatal("ordinary branch never completed")
	}
	pass.ResultOf = map[*analysis.Analyzer]any{resultfacts.Analyzer: resultfacts.NewEngine()}
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	child := pool.Within(setup + 10)
	if got := lockSuccessorStates(pass, state, child); len(got) != 0 || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("nested result cutoff kept %d successors; exhausted=%v/%v", len(got), child.Exhausted(), pool.Exhausted())
	}
	fresh := pool.Within(proofs.SummaryBudget)
	got := lockSuccessorStates(pass, state, fresh)
	if fresh.Exhausted() || len(got) != 1 || got[0].block != state.block.Succs[0] {
		t.Fatalf("fresh result branch: %+v", got)
	}
}

func TestLockWalkTerminationResultCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "locktermination", `package locktermination
 import "sync"
 var mu sync.Mutex
 var total int
 func stop(){`+strings.Repeat("total++\n", 30)+`panic("stop")}
 func root(){mu.Lock();stop()}
 `)
	fixture := newLockWalkFixture(pkg.Func("root"))
	// Cold result inference must share the caller's allowance. Warm summaries
	// may be cheaper, but cannot change the termination guarantee itself.
	limited := proofs.NewSearchBudget(30)
	if ok, reports, edges := fixture.run(limited); ok || !limited.Exhausted() || len(reports) != 0 || edges != 0 {
		t.Fatalf("termination cutoff complete=%v reports=%v edges=%d", ok, reports, edges)
	}
	for range 2 {
		fresh := proofs.NewSearchBudget(lockStateWorkBudget)
		if ok, reports, edges := fixture.run(fresh); !ok || fresh.Exhausted() || len(reports) != 0 || edges != 0 {
			t.Fatalf("fresh terminating path complete=%v reports=%v edges=%d", ok, reports, edges)
		}
	}
}
