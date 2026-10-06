package heapmodel

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestReturnedAliasAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returnalias", `package returnalias
func returned(value, other *int) (*int, *int) { return other, value }
`)
	fn := pkg.Func("returned")
	returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
	candidates := []ssa.Value{ssa.NewConst(nil, fn.Params[0].Type()), fn.Params[0]}
	for limit := range 7 {
		pool := proofs.NewSearchBudget(limit)
		budget := pool.Within(10)
		got := ReturnedMayAliasAnyWithin(returned, candidates, budget)
		if limit < 6 {
			if got || !budget.Exhausted() {
				t.Fatalf("allowance %d admitted incomplete alias census", limit)
			}
		} else if !got || budget.Exhausted() {
			t.Fatal("fresh complete query failed to recover the returned candidate")
		}
	}
	budget := proofs.NewSearchBudget(10)
	if ReturnedMayAliasAnyWithin(returned, []ssa.Value{ssa.NewConst(nil, fn.Params[0].Type())}, budget) || budget.Exhausted() {
		t.Fatal("unrelated candidate did not complete without alias evidence")
	}
}
