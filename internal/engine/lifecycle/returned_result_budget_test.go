package lifecycle

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestReturnedResultStorageAllowance(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
var external int
func cleanup(){}
func deferred()(value int){defer cleanup();return 3}
func opaque() int {return external}
func direct() int {return 4}
`)
	for _, name := range []string{"deferred", "opaque", "direct"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			for _, returned := range ssaflow.InstructionsOf[*ssa.Return](function) {
				if returned.Block().Comment == "recover" {
					continue
				}
				baseline := ReturnedResultWithin(returned, 0, nil)
				if baseline == nil {
					t.Fatal("default result unavailable")
				}
				if name == "opaque" && baseline != returned.Results[0] {
					t.Fatal("opaque storage did not retain original load")
				}
				checkReturnedResultAllowances(t, returned, baseline)
			}
		})
	}
}

func checkReturnedResultAllowances(t *testing.T, returned *ssa.Return, baseline ssa.Value) {
	t.Helper()
	complete := false
	for limit := range proofs.SummaryBudget {
		pool := proofs.NewSearchBudget(proofs.SummaryBudget)
		budget := pool.Within(limit)
		result := ReturnedResultWithin(returned, 0, budget)
		if budget.Exhausted() {
			if result != nil || pool.Exhausted() {
				t.Fatal("local cutoff published result or exhausted pool")
			}
			if fresh := ReturnedResultWithin(returned, 0, pool.Within(proofs.SummaryBudget)); fresh != baseline {
				t.Fatal("fresh query did not recover default result")
			}
			continue
		}
		if result != baseline {
			t.Fatalf("result=%v; want %v", result, baseline)
		}
		complete = true
		break
	}
	if !complete {
		t.Fatal("result query never completed")
	}
}
