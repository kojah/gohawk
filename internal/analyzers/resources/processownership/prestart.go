package processownership

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// All pre-Start policies consume one completed structural instruction census.
// A cutoff discards its prefix, so no owner registration or caller store can be
// inferred from shortened discovery. Binding/heap query costs remain separate.
type processStartInstructions struct {
	ssaflow.Proof
	instructions []ssa.Instruction
}

func collectProcessStartInstructions(start *ssa.Call, budget *ssaflow.SearchBudget) processStartInstructions {
	var before []ssa.Instruction
	for instruction := range ssaflow.InstructionsStrictlyDominatingWithin(start, budget) {
		before = append(before, instruction)
	}
	if budget.Exhausted() {
		return processStartInstructions{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}}
	}
	return processStartInstructions{Proof: ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceStructuralWalk}, instructions: before}
}
