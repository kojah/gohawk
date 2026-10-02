package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Exact cleanup crosses only the selected identity wrappers. A phi or load
// cannot become the parameter just because its possible origins contain it.
func TestExactCleanupReceiverAllowance(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture+`
 type same resource
 func observe(interface{}){}
 func wrapped(p *resource){observe(p)}
 func converted(p *resource){observe((*same)(p))}
 func merged(p,q *resource,flag bool){v:=p;if flag{v=q};observe(v)}
 func loaded(p **resource){observe(*p)}
 `)
	for _, test := range []struct {
		name string
		want bool
	}{{"wrapped", true}, {"converted", true}, {"merged", false}, {"loaded", false}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			var receiver ssa.Value
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
				if ssaflow.CallName(call.Common()) == "observe" {
					receiver = call.Common().Args[0]
				}
			}
			if receiver == nil {
				t.Fatal("receiver not found")
			}
			if test.want {
				cut := ssaflow.NewSearchBudget(1)
				if exactCleanupReceiver(receiver, fn.Params[0], cut) || !cut.Exhausted() {
					t.Fatal("wrapper traversal bypassed allowance")
				}
			}
			for limit := 1; limit <= ssaflow.QueryBudget; limit++ {
				budget := ssaflow.NewSearchBudget(limit)
				got := exactCleanupReceiver(receiver, fn.Params[0], budget)
				if budget.Exhausted() {
					if got {
						t.Fatal("cut proves exact receiver")
					}
					continue
				}
				if got != test.want {
					t.Fatalf("complete %d match=%v", limit, got)
				}
				return
			}
			t.Fatal("receiver query never completed")
		})
	}
}
