package goroutineownership

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestDominatingCensusCutoff(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	pkg := ssaflowtest.BuildPackage(t, "spawncensus", `package spawncensus
 func subject(){done:=make(chan int);go func(){close(done)}();<-done}
 `)
	fn := pkg.Func("subject")
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
	candidate := newSpawnAnalysis(pass, fn, ssaflow.InstructionsOf[*ssa.Go](fn)[0])
	candidate.pool = proofs.NewSearchBudget(0).Observed(candidate.probe.Observer())
	proof, decided := candidate.dominatingProof()
	if !decided || proof.Outcome != GoroutineUnknown || proof.Reason != reasonPreSpawnCensusCutoff {
		t.Fatalf("cutoff: %+v decided=%v", proof, decided)
	}
	requireCensusCutoffTrace(t, path, "pre-spawn-census")
	candidate.pool = proofs.NewSearchBudget(spawnPoolBudget)
	if proof, decided := candidate.dominatingProof(); decided {
		t.Fatalf("fresh prefix invented cleanup: %+v", proof)
	}
}
