package ssaflow

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestDeferredTerminationAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "termination", `package termination
import "os"
func marker() {}
func unconditional() { marker(); defer os.Exit(0); marker() }
func conditional(flag bool) { if flag { defer os.Exit(0) }; marker() }
func unrelated() { defer marker(); marker() }
`)
	for _, name := range []string{"unconditional", "conditional", "unrelated"} {
		t.Run(name, func(t *testing.T) {
			runs := InstructionsOf[*ssa.RunDefers](pkg.Func(name))
			if len(runs) == 0 {
				t.Fatal("expected actual SSA deferred execution")
			}
			for _, run := range runs {
				fresh := proofs.NewSearchBudget(proofs.QueryBudget)
				got := InstructionTerminatesWithin(run, nil, fresh)
				if got != (name == "unconditional") || got != InstructionTerminatesWith(run, nil) || fresh.Exhausted() {
					t.Fatal("fresh query must preserve conditional registration and catalog policy")
				}
				used := proofs.QueryBudget - fresh.Remaining()
				for limit := range used {
					cut := proofs.NewSearchBudget(limit)
					if InstructionTerminatesWithin(run, nil, cut) || !cut.Exhausted() {
						t.Fatalf("limit %d admitted termination or missed interrupted work", limit)
					}
				}
				pool := proofs.NewSearchBudget(0)
				if InstructionTerminatesWithin(run, nil, pool.Within(proofs.QueryBudget)) || !pool.Exhausted() {
					t.Fatal("candidate-pool cutoff cannot establish termination")
				}
			}
		})
	}
}

func TestTerminationCallbackAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "termination", `package termination
func marker() {}
func subject() { marker() }
`)
	call := InstructionsOf[*ssa.Call](pkg.Func("subject"))[0]
	zero := proofs.NewSearchBudget(0)
	called := false
	if InstructionTerminatesWithin(call, func(*ssa.Call) bool { called = true; return true }, zero) || called || !zero.Exhausted() {
		t.Fatal("call dispatch must spend before consulting a callback")
	}
	budget := proofs.NewSearchBudget(proofs.QueryBudget)
	if InstructionTerminatesWithin(call, func(*ssa.Call) bool {
		for budget.Spend() {
		}
		return true
	}, budget) || !budget.Exhausted() {
		t.Fatal("an interrupted callback cannot establish termination")
	}
	if !InstructionTerminatesWithin(call, func(*ssa.Call) bool { return true }, proofs.NewSearchBudget(proofs.QueryBudget)) {
		t.Fatal("a fresh callback still establishes termination")
	}
}

func TestDeferredTerminationCutoffKeepsFlowUnknown(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "termination", `package termination
import "os"
func marker() {}
func subject() { marker(); defer os.Exit(0); marker() }
`)
	function := pkg.Func("subject")
	budget := proofs.NewSearchBudget(proofs.QueryBudget)
	called := false
	flow := ObligationFlow{Budget: budget, Instruction: func(instruction ssa.Instruction) ObligationAction {
		if _, ok := instruction.(*ssa.RunDefers); ok {
			called = true
			// Leave one visit for the nested census, so its next instruction
			// cuts off without interrupting this classifier's own answer.
			for budget.Remaining() > 1 {
				budget.Spend()
			}
		}
		return ObligationNone
	}}
	if got := EvaluateObligationFromEntry(function, flow); got != ObligationUncertain || !called || !budget.Exhausted() {
		t.Fatal("interrupted deferred termination cannot prove honored coverage")
	}
	flow.Budget = proofs.NewSearchBudget(proofs.QueryBudget)
	flow.Instruction = func(ssa.Instruction) ObligationAction { return ObligationNone }
	if got := EvaluateObligationFromEntry(function, flow); got != ObligationHonored || flow.Budget.Exhausted() {
		t.Fatal("fresh deferred exit must retain the default honored outcome")
	}
}
