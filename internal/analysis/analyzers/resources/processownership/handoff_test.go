package processownership

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"github.com/kojah/gohawk/internal/testsupport/analyzertest"
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
		pool := proofs.NewSearchBudget(processPoolBudget)
		cut := provePossibleWaitHandoff(instruction, fn.Params[0], pool.Within(0))
		if cut.Reason != proofs.EvidenceBudgetExhausted || cut.State != proofs.EvidenceUnknown || pool.Exhausted() {
			t.Fatalf("%s cutoff: %+v", name, cut)
		}
		fresh := provePossibleWaitHandoff(instruction, fn.Params[0], pool.Within(processQueryBudget))
		want := proofs.EvidenceDisproven
		if name == "endless" {
			want = proofs.EvidenceUnknown
		}
		if fresh.State != want || fresh.Reason == proofs.EvidenceBudgetExhausted {
			t.Fatalf("%s fresh: %+v", name, fresh)
		}
	}
}
