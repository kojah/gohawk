package lifecycle

import (
	"reflect"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestResultGuardNamedCellCensusReuse(t *testing.T) {
	fn := buildTestSSA(t, `package ssaflowtest
 func repeated()(err error){
  defer func(){println(err)}()
  defer func(){println(err)}()
  defer func(){println(err)}()
  return nil
 }
 `).Func("repeated")
	closures := ssaflow.InstructionsOf[*ssa.MakeClosure](fn)
	if len(closures) != 3 {
		t.Fatalf("closures=%d", len(closures))
	}
	pool := proofs.NewSearchBudget(10000)
	var named ssaflow.NamedResultCellsProof
	first := capturedResultCells(fn, closures[0], pool.Within(proofs.SummaryBudget), &named)
	if !named.Proven() || len(first) != 1 || len(named.Cells) != 1 {
		t.Fatalf("first=%v census=%+v", first, named)
	}
	for _, closure := range closures[1:] {
		budget := pool.Within(len(closure.Bindings))
		cells := capturedResultCells(fn, closure, budget, &named)
		if budget.Exhausted() || pool.Exhausted() || !reflect.DeepEqual(cells, first) {
			t.Fatalf("warm=%v first=%v exhausted=%v", cells, first, budget.Exhausted())
		}
	}
	var cold ssaflow.NamedResultCellsProof
	cut := pool.Within(3)
	if cells := capturedResultCells(fn, closures[0], cut, &cold); len(cells) != 0 || cold.Proven() || cold.Cells != nil || !cut.Exhausted() {
		t.Fatalf("cut=%v census=%+v", cells, cold)
	}
	fresh := pool.Within(proofs.SummaryBudget)
	cells := capturedResultCells(fn, closures[0], fresh, &cold)
	if fresh.Exhausted() || pool.Exhausted() || !reflect.DeepEqual(cells, first) || !reflect.DeepEqual(cold, named) {
		t.Fatalf("fresh=%v census=%+v", cells, cold)
	}
}
