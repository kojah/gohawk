package processownership

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestProcessStartCensusCutoff(t *testing.T) {
	pkg := startCensusPackage(t)
	fn := pkg.Func("subject")
	start := startupTestCall(t, fn)
	pool := proofs.NewSearchBudget(processPoolBudget)
	for limit := range 1000 {
		child := pool.Within(limit)
		result := collectProcessStartInstructions(start, ssaflow.CallReceiver(start.Common()), child)
		if result.Proven() {
			if child.Exhausted() || len(result.instructions) == 0 || len(result.owners) != 3 {
				t.Fatal("incomplete published census")
			}
			return
		}
		unknownCutoff := result.State == proofs.EvidenceUnknown && result.Reason == proofs.EvidenceBudgetExhausted
		publishedPrefix := len(result.instructions) != 0 || len(result.owners) != 0
		if !unknownCutoff || publishedPrefix || pool.Exhausted() {
			t.Fatalf("cutoff: %+v", result)
		}
		fresh := collectProcessStartInstructions(start, ssaflow.CallReceiver(start.Common()), pool.Within(processQueryBudget))
		if !fresh.Proven() || len(fresh.instructions) == 0 || len(fresh.owners) != 3 {
			t.Fatal("fresh child lost census")
		}
	}
	t.Fatal("census never completed")
}

func TestRegisteredOwnerAllowance(t *testing.T) {
	fn := startCensusPackage(t).Func("subject")
	start := startupTestCall(t, fn)
	command := ssaflow.CallReceiver(start.Common())
	var before []ssa.Instruction
	for instruction := range cfg.InstructionsStrictlyDominatingWithin(start, nil) {
		before = append(before, instruction)
	}
	pool := proofs.NewSearchBudget(processPoolBudget)
	child := pool.Within(len(before))
	if owners := processOwnersRegisteredBefore(before, command, child); len(owners) != 0 || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("owner metadata bypassed body-only allowance: %v", owners)
	}
	if owners := processOwnersRegisteredBefore(before, command, pool.Within(processQueryBudget)); len(owners) != 3 {
		t.Fatalf("fresh multi-result owner inventory: %v", owners)
	}
	if result := collectProcessStartInstructions(nil, nil, nil); result.State != proofs.EvidenceUnknown || result.Reason != proofs.EvidenceUnavailable {
		t.Fatalf("nil Start invented completed census: %+v", result)
	}
}

func startCensusPackage(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "startcensus", `package startcensus
 import "os/exec"
 type holder struct{cmd *exec.Cmd}
 func hold(cmd *exec.Cmd)(*holder,func()){return &holder{cmd},func(){cmd.Wait()}}
 func subject(flag bool){cmd:=exec.Command("tool");owner,cleanup:=hold(cmd);defer cleanup()
 if flag {hold(cmd)};cmd.Start();println(owner)}
 `)
}
