package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestDeferredCellMappingSharesRequestAllowance(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture+`
func deferredCleared(p *resource, pick bool){held:=p;defer func(){if held!=nil{held.Close()}}();if pick{p.Close();held=nil}}
`)
	function := pkg.Func("deferredCleared")
	deferred := ssaflow.InstructionsOf[*ssa.Defer](function)[0]
	body, closure := ssaflow.DirectCallee(deferred.Common())
	if closure == nil || len(closure.Bindings) != 1 {
		t.Fatal("expected captured deferred cell")
	}
	cell, ok := closure.Bindings[0].(*ssa.Alloc)
	if !ok {
		t.Fatalf("capture is %T, want actual cell", closure.Bindings[0])
	}
	pool := ssaflow.NewSearchBudget(10 * ssaflow.QueryBudget)
	search := newCompletionSearch("Close", CoverageEveryReturn, pool.Within(1))
	if _, mapped := search.deferredCellLocal(body.FreeVars[0], cell, function.Params[0], deferred); mapped || !search.budget.Exhausted() {
		t.Fatal("graph relation bypassed the exhausted mapping allowance")
	}
	if pool.Exhausted() {
		t.Fatal("independent child cutoff exhausted the pool")
	}
	search.budget = pool.Within(ssaflow.QueryBudget)
	local, mapped := search.deferredCellLocal(body.FreeVars[0], cell, function.Params[0], deferred)
	if !mapped || local.kind != localExact || search.budget.Exhausted() {
		t.Fatalf("fresh exact cell mapping did not recover: %+v, mapped=%v", local, mapped)
	}
}
