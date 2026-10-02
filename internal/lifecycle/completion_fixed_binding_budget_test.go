package lifecycle

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
)

func TestCompletionFixedBindingCutoff(t *testing.T) {
	source := completionCoverageBudgetFixture + `
 func many(p *resource,flag *int){p.Close();` + strings.Repeat("println(flag);", ssaflow.QueryBudget+1) + `if flag==nil{println("nil")}}
 func runMany(p *resource){many(p,nil)}
 `
	fn := buildTestSSA(t, source).Func("runMany")
	call := findLaunch(t, fn)
	pool := ssaflow.NewSearchBudget(10 * ssaflow.SummaryBudget)
	child := pool.Within(ssaflow.QueryBudget)
	request := CompletionRequest{Instruction: call, Target: fn.Params[0], Methods: []string{"Close"}, Coverage: CoverageAnywhere, Budget: child}
	proof := ProveCompletion(request)
	if proof.State != ssaflow.EvidenceUnknown || proof.Reason != ssaflow.EvidenceBudgetExhausted || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("binding child=%+v, parent exhausted=%v", proof, pool.Exhausted())
	}
	request.Budget = pool.Within(2 * ssaflow.SummaryBudget)
	if proof := ProveCompletion(request); !proof.Proven() {
		t.Fatalf("fresh completion=%+v", proof)
	}
	search := newCompletionSearch("Close", CoverageAnywhere, pool.Within(ssaflow.QueryBudget))
	if got := search.completes(call, fn.Params[0]); got.proven || !search.budget.Exhausted() {
		t.Fatalf("memo child=%+v", got)
	}
	search.budget = pool.Within(2 * ssaflow.SummaryBudget)
	if got := search.completes(call, fn.Params[0]); !got.proven {
		t.Fatalf("fresh memo=%+v", got)
	}
}
