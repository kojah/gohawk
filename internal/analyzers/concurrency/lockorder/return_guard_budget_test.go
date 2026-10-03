package lockorder

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestLockReturnGuardAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returnguard", `package returnguard
func guarded(err error) error { if err != nil { return nil }; return err }
func unrelated(err, other error) error { if other != nil { return nil }; return err }
`)
	for _, name := range []string{"guarded", "unrelated"} {
		fn := pkg.Func(name)
		setup := &lockFunctionSetup{branches: ssaflow.InstructionsOf[*ssa.If](fn)}
		var returned *ssa.Return
		for _, candidate := range ssaflow.InstructionsOf[*ssa.Return](fn) {
			if candidate.Results[0] == fn.Params[0] {
				returned = candidate
			}
		}
		if returned == nil {
			t.Fatal("actual SSA must return the checked value directly")
		}
		if name == "guarded" {
			pool := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
			query := lockReturnQueries{setup: setup, budget: pool.Within(3)}
			if query.nilGuardDominatesReturn(fn.Params[0], returned) || !query.budget.Exhausted() || pool.Exhausted() {
				t.Fatal("guard identity bypassed the selection-only allowance")
			}
		}
		complete := false
		for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
			query := lockReturnQueries{setup: setup, budget: ssaflow.NewSearchBudget(limit)}
			got := query.nilGuardDominatesReturn(fn.Params[0], returned)
			if query.budget.Exhausted() {
				if got {
					t.Fatal("interrupted guard supplied a return contract")
				}
				continue
			}
			if got != (name == "guarded") {
				t.Fatalf("complete %s guard = %v", name, got)
			}
			complete = true
			break
		}
		if !complete {
			t.Fatal("fresh guard never completed")
		}
	}
}
