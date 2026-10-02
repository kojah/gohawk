package processownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestProcessStartCensusCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "startcensus", `package startcensus
 import "os/exec"
 func hold(*exec.Cmd){}
 func subject(flag bool){cmd:=exec.Command("tool");hold(cmd)
 if flag {hold(cmd)};cmd.Start();hold(cmd)}
 `)
	fn := pkg.Func("subject")
	var start *ssa.Call
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if candidate, _, ok := startedCommand(call); ok {
			start = candidate
		}
	}
	if start == nil {
		t.Fatal("missing actual Start")
	}
	pool := ssaflow.NewSearchBudget(processPoolBudget)
	for limit := range 1000 {
		child := pool.Within(limit)
		result := collectProcessStartInstructions(start, child)
		if result.Proven() {
			if child.Exhausted() || len(result.instructions) == 0 {
				t.Fatal("incomplete published census")
			}
			return
		}
		if result.State != ssaflow.EvidenceUnknown || result.Reason != ssaflow.EvidenceBudgetExhausted || len(result.instructions) != 0 || pool.Exhausted() {
			t.Fatalf("cutoff: %+v", result)
		}
		fresh := collectProcessStartInstructions(start, pool.Within(processQueryBudget))
		if !fresh.Proven() || len(fresh.instructions) == 0 {
			t.Fatal("fresh child lost census")
		}
	}
	t.Fatal("census never completed")
}
