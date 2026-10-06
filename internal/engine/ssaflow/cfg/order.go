package cfg

import (
	"slices"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"golang.org/x/tools/go/ssa"
)

// InstructionIndex returns instruction position within its basic block.
func InstructionIndex(instruction ssa.Instruction) int {
	return InstructionIndexWithin(instruction, nil)
}

// InstructionIndexWithin charges each inspected instruction before selecting
// its block position. At cutoff -1 is unavailable; callers inspect the budget.
// A nil budget retains the default scan.
func InstructionIndexWithin(instruction ssa.Instruction, budget *proofs.SearchBudget) int {
	for index, candidate := range instruction.Block().Instrs {
		if !budget.Spend() {
			return -1
		}
		if candidate == instruction {
			return index
		}
	}
	return -1
}

// Same-block dominance and possible-follow queries use one instruction order
// decision. An unindexed instruction never contributes ordering evidence.
func instructionOrderedWithin(before, after ssa.Instruction, budget *proofs.SearchBudget) bool {
	first := InstructionIndexWithin(before, budget)
	if first < 0 {
		return false
	}
	last := InstructionIndexWithin(after, budget)
	return last >= 0 && !budget.Exhausted() && first <= last
}

// InstructionDominates reports whether every path to after executes before.
// Instruction order is respected when both values belong to one block.
func InstructionDominates(before, after ssa.Instruction) bool {
	return InstructionDominatesWithin(before, after, nil)
}

// InstructionDominatesWithin shares the allowance across same-block indexing
// or a constant-time dominator-tree comparison. False at cutoff is unavailable,
// not evidence of an uncovered path. A nil budget retains default order policy.
func InstructionDominatesWithin(before, after ssa.Instruction, budget *proofs.SearchBudget) bool {
	if before == nil || after == nil || before.Parent() != after.Parent() {
		return false
	}
	if before.Block() == after.Block() {
		return instructionOrderedWithin(before, after, budget)
	}
	return budget.Spend() && before.Block().Dominates(after.Block())
}

// InstructionMayFollow reports whether after is reachable after before. This
// is intentionally weaker than dominance and is used only to reject evidence
// that is earlier than, or disconnected from, the obligation it purports to
// settle.
func InstructionMayFollow(before, after ssa.Instruction) bool {
	return InstructionMayFollowWithin(before, after, nil)
}

// InstructionMayFollowWithin applies the same order/reachability policy under
// budget. A false result at exhaustion is unavailable, not disconnection.
func InstructionMayFollowWithin(before, after ssa.Instruction, budget *proofs.SearchBudget) bool {
	if before == nil || after == nil || before.Parent() != after.Parent() {
		return false
	}
	if before.Block() == after.Block() {
		return instructionOrderedWithin(before, after, budget)
	}
	return blockReachableFromWithin(before.Block().Succs, after.Block(), budget)
}

// BlockReachable reports whether target is reachable from within their
// shared function. A block is reachable from itself without traversing an edge.
func BlockReachable(from, target *ssa.BasicBlock) bool {
	return BlockReachableWithin(from, target, nil)
}

// BlockReachableWithin shares the allowance with the existing CFG traversal.
// False at cutoff means unavailable, not proof that the target is unreachable.
func BlockReachableWithin(from, target *ssa.BasicBlock, budget *proofs.SearchBudget) bool {
	if from == nil || target == nil || from.Parent() != target.Parent() {
		return false
	}
	return blockReachableFromWithin([]*ssa.BasicBlock{from}, target, budget)
}

// BlockInCycle reports whether control flow can return to start.
func BlockInCycle(start *ssa.BasicBlock) bool {
	return BlockInCycleWithin(start, nil)
}

// BlockInCycleWithin shares queued CFG visits with budget. False at cutoff
// is unavailable, not proof of acyclic execution; callers check the allowance.
func BlockInCycleWithin(start *ssa.BasicBlock, budget *proofs.SearchBudget) bool {
	// Starting at successors requires at least one edge. Starting at the block
	// itself would incorrectly classify every acyclic block as a cycle.
	return blockReachableFromWithin(start.Succs, start, budget)
}

// blockReachableFromWithin owns raw CFG traversal; callers choose whether the
// initial block or only its successors can count. Clone the seeds because
// queue growth must not overwrite an SSA block's successor backing array.
func blockReachableFromWithin(seeds []*ssa.BasicBlock, target *ssa.BasicBlock, budget *proofs.SearchBudget) bool {
	seen := map[*ssa.BasicBlock]bool{}
	queue := slices.Clone(seeds)
	head := 0
	for head < len(queue) {
		block := queue[head]
		head++
		// A drained queue can reuse its owned backing array for successors.
		if head == len(queue) {
			queue = queue[:0]
			head = 0
		}
		if !budget.Spend() {
			return false
		}
		if block == target {
			return true
		}
		if seen[block] {
			continue
		}
		seen[block] = true
		for _, successor := range block.Succs {
			if !budget.Spend() {
				return false
			}
			if len(queue) == cap(queue) && head > 0 {
				queue = queue[:copy(queue, queue[head:])]
				head = 0
			}
			queue = append(queue, successor)
		}
	}
	return false
}
