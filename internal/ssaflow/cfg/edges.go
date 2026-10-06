package cfg

import (
	proofs "github.com/kojah/gohawk/internal/proof"
	"golang.org/x/tools/go/ssa"
)

// KeepSuccessorWithin selects one edge from an already-feasible set. It does
// not establish feasibility itself. Cutoff admits no partially inspected set.
func KeepSuccessorWithin(successors []*ssa.BasicBlock, taken *ssa.BasicBlock, budget *proofs.SearchBudget) []*ssa.BasicBlock {
	for _, successor := range successors {
		if !budget.Spend() {
			return nil
		}
		if successor == taken {
			return []*ssa.BasicBlock{successor}
		}
	}
	return nil
}
