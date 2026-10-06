package ssaflow

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestInstructionOrderAndIndexBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "order", `package order
 func marker(value int) {}
 func subject(flag bool) { marker(1); marker(2); if flag { marker(3) }; marker(4) }
`)
	calls := InstructionsOf[*ssa.Call](pkg.Func("subject"))
	for _, before := range calls {
		zero := proofs.NewSearchBudget(0)
		if cfg.InstructionIndexWithin(before, zero) != -1 || !zero.Exhausted() {
			t.Fatal("initial position cutoff must remain unavailable")
		}
		fresh := proofs.NewSearchBudget(proofs.QueryBudget)
		if cfg.InstructionIndexWithin(before, fresh) != cfg.InstructionIndex(before) || fresh.Exhausted() {
			t.Fatal("fresh index must preserve actual SSA position")
		}
		for _, after := range calls {
			zero := proofs.NewSearchBudget(0)
			if cfg.InstructionDominatesWithin(before, after, zero) || !zero.Exhausted() {
				t.Fatal("same/cross-block dominance must charge before evidence")
			}
			fresh := proofs.NewSearchBudget(proofs.QueryBudget)
			if cfg.InstructionDominatesWithin(before, after, fresh) != cfg.InstructionDominates(before, after) || fresh.Exhausted() {
				t.Fatal("fresh dominance changed direction or block order")
			}
		}
	}
	pool := proofs.NewSearchBudget(1)
	shared := pool.Within(proofs.QueryBudget)
	if cfg.InstructionDominatesWithin(calls[0], calls[1], shared) || !shared.PoolExhausted() {
		t.Fatal("both same-block positions must share the candidate pool")
	}
}

func TestObligationInitialLookupCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "setup", `package setup
 func marker(value int) {}
 func subject() { marker(1); marker(2); marker(3); marker(4) }
`)
	calls := InstructionsOf[*ssa.Call](pkg.Func("subject"))
	for _, action := range []ObligationAction{ObligationNone, ObligationExact} {
		classified := 0
		flow := ObligationFlow{Start: calls[2], Budget: proofs.NewSearchBudget(2), Instruction: func(ssa.Instruction) ObligationAction {
			classified++
			return action
		}}
		outcome, witness := EvaluateObligationWitness(flow)
		if outcome != ObligationUncertain || witness != nil || classified != 0 || !flow.Budget.Exhausted() {
			t.Fatal("incomplete setup cannot classify, honor or violate an obligation")
		}
		flow.Budget = proofs.NewSearchBudget(proofs.QueryBudget)
		got, witness := EvaluateObligationWitness(flow)
		want := ObligationHonored
		if action == ObligationNone {
			want = ObligationViolated
		}
		if got != want || flow.Budget.Exhausted() || (witness != nil) != (want == ObligationViolated) {
			t.Fatal("fresh setup lost exact coverage or its uncovered-return witness")
		}
	}
}
