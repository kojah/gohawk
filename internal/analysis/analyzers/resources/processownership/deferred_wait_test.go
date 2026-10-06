package processownership

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
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
		want proofs.EvidenceState
	}{
		{"exact", proofs.EvidenceProven},
		{"conditional", proofs.EvidenceDisproven},
		{"guarded", proofs.EvidenceUnknown},
		{"replaced", proofs.EvidenceDisproven},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			deferred := ssaflow.InstructionsOf[*ssa.Defer](fn)[0]
			command := fn.Params[0]
			pool := proofs.NewSearchBudget(processPoolBudget)
			for limit := range 1000 {
				budget := pool.Within(limit)
				got := deferredClosureWaitsForCommand(deferred, command, budget)
				if !budget.Exhausted() {
					if got != test.want {
						t.Fatalf("complete limit %d: state=%v want=%v", limit, got, test.want)
					}
					return
				}
				if got != proofs.EvidenceUnknown || pool.Exhausted() {
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
	search := deferredWaitSearch{function: callback, budget: proofs.NewSearchBudget(0)}
	if result := search.coverage(callback.FreeVars[0], callback.FreeVars[0]); result.State != proofs.EvidenceUnknown || !search.budget.Exhausted() {
		t.Fatalf("coverage bypassed allowance: %+v", result)
	}
	proof := &commandProof{pool: proofs.NewSearchBudget(0)}
	if state := processOwnershipAction(proof, deferred, fn.Params[0]); state != proofs.EvidenceUnknown {
		t.Fatalf("classifier bypassed candidate allowance: %v", state)
	}
}
