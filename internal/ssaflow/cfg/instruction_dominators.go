package cfg

import (
	"iter"

	proofs "github.com/kojah/gohawk/internal/proof"
	"golang.org/x/tools/go/ssa"
)

// InstructionsStrictlyDominatingWithin yields instructions that structurally
// dominate at, excluding at itself, in function block order. Instructions later
// in at's block are excluded even in loops. Block checks, indexing and yielded
// visits share budget; a cutoff cannot prove absent dominating instructions.
// Callers needing a complete census must discard its prefix at cutoff. Breaking
// on a positive witness avoids later work. Nil budget retains unbounded policy.
func InstructionsStrictlyDominatingWithin(at ssa.Instruction, budget *proofs.SearchBudget) iter.Seq[ssa.Instruction] {
	return func(yield func(ssa.Instruction) bool) {
		if at == nil || at.Block() == nil || at.Parent() == nil {
			return
		}
		index := InstructionIndexWithin(at, budget)
		if index < 0 {
			return
		}
		for _, block := range at.Parent().Blocks {
			if !budget.Spend() {
				return
			}
			if !block.Dominates(at.Block()) {
				continue
			}
			instructions := block.Instrs
			if block == at.Block() {
				instructions = instructions[:index]
			}
			for _, instruction := range instructions {
				if !budget.Spend() || !yield(instruction) {
					return
				}
			}
		}
	}
}
