package lockorder

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestReturnedSuccessCellSharesAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returnstorage", `package returnstorage
func cleanup() {}
func subject() (ok bool) { defer cleanup(); return true }
func declined() (ok bool) { defer cleanup(); return false }
func plain() bool { return true }
`)
	for _, name := range []string{"subject", "declined", "plain"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			var dump strings.Builder
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			var returned *ssa.Return
			for _, candidate := range ssaflow.InstructionsOf[*ssa.Return](function) {
				if candidate.Block().Comment != "recover" {
					returned = candidate
					break
				}
			}
			if returned == nil {
				t.Fatal("no normal return")
			}
			for _, limit := range []int{0, 1, proofs.SummaryBudget} {
				budget := proofs.NewSearchBudget(limit)
				query := lockReturnQueries{budget: budget}
				success := query.successfulReturn(function, returned)
				if limit == 0 && (!budget.Exhausted() || success) {
					t.Fatalf("zero allowance supplied success=%v exhausted=%v", success, budget.Exhausted())
				}
				if budget.Exhausted() && success {
					t.Fatal("interrupted result storage supplied success")
				}
				if limit == proofs.SummaryBudget && (budget.Exhausted() || success != (name != "declined")) {
					t.Fatalf("complete success=%v exhausted=%v", success, budget.Exhausted())
				}
			}
		})
	}
}
