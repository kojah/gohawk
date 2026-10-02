package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestNamedResultCellAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "namedresult", `package namedresult
 func named(flag bool)(n int,err error){defer func(){println(err)}();if flag{return 1,nil};return 2,nil}
 `)
	fn := pkg.Func("named")
	var cell *ssa.Alloc
	for _, value := range ssaflow.InstructionsOf[*ssa.Alloc](fn) {
		if slot, ok := ssaflow.NamedResultCellWithin(fn, value, nil); ok && slot == 1 {
			cell = value
			break
		}
	}
	if cell == nil {
		t.Fatal("missing named error cell")
	}
	for limit := range 100 {
		budget := ssaflow.NewSearchBudget(limit)
		slot, ok := ssaflow.NamedResultCellWithin(fn, cell, budget)
		if budget.Exhausted() || budget.PoolExhausted() {
			if ok {
				t.Fatalf("cut %d identified result slot %d", limit, slot)
			}
			continue
		}
		if !ok || slot != 1 {
			t.Fatalf("complete cell = %d/%v", slot, ok)
		}
		return
	}
	t.Fatal("named result search never completed")
}
