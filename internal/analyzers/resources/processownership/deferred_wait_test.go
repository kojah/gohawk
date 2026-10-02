package processownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestDeferredWaitAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "deferredwait", `package deferredwait
import "os/exec"
func exact(cmd *exec.Cmd) {defer func(){cmd.Wait()}()}
func conditional(cmd *exec.Cmd, flag bool) {defer func(){if flag {cmd.Wait()}}()}
func guarded(cmd *exec.Cmd) {defer func(){if cmd.Process != nil {cmd.Wait()}}()}
func replaced(cmd *exec.Cmd) {defer func(){cmd.Process=nil;if cmd.Process != nil {cmd.Wait()}}()}
`)
	for _, test := range []struct {
		name string
		want ssaflow.EvidenceState
	}{
		{"exact", ssaflow.EvidenceProven},
		{"conditional", ssaflow.EvidenceDisproven},
		{"guarded", ssaflow.EvidenceUnknown},
		{"replaced", ssaflow.EvidenceDisproven},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			deferred := ssaflow.InstructionsOf[*ssa.Defer](fn)[0]
			command := fn.Params[0]
			pool := ssaflow.NewSearchBudget(processPoolBudget)
			for limit := range 1000 {
				budget := pool.Within(limit)
				got := deferredClosureWaitsForCommand(deferred, command, budget)
				if !budget.Exhausted() {
					if got != test.want {
						t.Fatalf("complete limit %d: state=%v want=%v", limit, got, test.want)
					}
					return
				}
				if got != ssaflow.EvidenceUnknown || pool.Exhausted() {
					t.Fatalf("cut limit %d: state=%v pool exhausted=%v", limit, got, pool.Exhausted())
				}
				fresh := pool.Within(processQueryBudget)
				if got := deferredClosureWaitsForCommand(deferred, command, fresh); got != test.want || fresh.Exhausted() {
					t.Fatalf("fresh after limit %d: state=%v want=%v", limit, got, test.want)
				}
			}
			t.Fatal("search did not finish")
		})
	}
	fn := pkg.Func("exact")
	deferred := ssaflow.InstructionsOf[*ssa.Defer](fn)[0]
	callback := fn.AnonFuncs[0]
	search := deferredWaitSearch{function: callback, budget: ssaflow.NewSearchBudget(0)}
	if result := search.coverage(callback.FreeVars[0], callback.FreeVars[0]); result.State != ssaflow.EvidenceUnknown || !search.budget.Exhausted() {
		t.Fatalf("coverage bypassed allowance: %+v", result)
	}
	proof := &commandProof{pool: ssaflow.NewSearchBudget(0)}
	if state := processOwnershipAction(proof, deferred, fn.Params[0]); state != ssaflow.EvidenceUnknown {
		t.Fatalf("classifier bypassed candidate allowance: %v", state)
	}
}
