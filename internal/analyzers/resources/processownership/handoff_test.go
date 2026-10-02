package processownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

func TestProcessClosureChoices(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "processchoices")
}

func TestPossibleNonreturningWaitAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "waitreach", `package waitreach
 import "os/exec"
 func endless(cmd *exec.Cmd){go func(){cmd.Wait();for{}}()}
 func returning(cmd *exec.Cmd){go func(){cmd.Wait()}()}
 func unrelated(cmd *exec.Cmd){go func(){for{}}()}
 `)
	for _, name := range []string{"endless", "returning", "unrelated"} {
		fn := pkg.Func(name)
		instruction := ssaflow.InstructionsOf[*ssa.Go](fn)[0]
		pool := ssaflow.NewSearchBudget(processPoolBudget)
		cut := provePossibleWaitHandoff(instruction, fn.Params[0], pool.Within(0))
		if cut.Reason != ssaflow.EvidenceBudgetExhausted || cut.State != ssaflow.EvidenceUnknown || pool.Exhausted() {
			t.Fatalf("%s cutoff: %+v", name, cut)
		}
		fresh := provePossibleWaitHandoff(instruction, fn.Params[0], pool.Within(processQueryBudget))
		want := ssaflow.EvidenceDisproven
		if name == "endless" {
			want = ssaflow.EvidenceUnknown
		}
		if fresh.State != want || fresh.Reason == ssaflow.EvidenceBudgetExhausted {
			t.Fatalf("%s fresh: %+v", name, fresh)
		}
	}
}
