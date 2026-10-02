package ssaflow

import (
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/ssa"
)

// Control-flow evidence answers path-sensitive ownership questions shared by
// several analyzers. Traversal retains the predecessor for phi and branch
// feasibility, and treats only reachable normal returns as lifecycle exits.

// InstructionIndex returns instruction position within its basic block.
func InstructionIndex(instruction ssa.Instruction) int {
	return InstructionIndexWithin(instruction, nil)
}

// InstructionIndexWithin charges each inspected instruction before selecting
// its block position. At cutoff -1 is unavailable; callers inspect the budget.
// A nil budget retains the default scan.
func InstructionIndexWithin(instruction ssa.Instruction, budget *SearchBudget) int {
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
func instructionOrderedWithin(before, after ssa.Instruction, budget *SearchBudget) bool {
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
func InstructionDominatesWithin(before, after ssa.Instruction, budget *SearchBudget) bool {
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
func InstructionMayFollowWithin(before, after ssa.Instruction, budget *SearchBudget) bool {
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
func BlockReachableWithin(from, target *ssa.BasicBlock, budget *SearchBudget) bool {
	if from == nil || target == nil || from.Parent() != target.Parent() {
		return false
	}
	return blockReachableFromWithin([]*ssa.BasicBlock{from}, target, budget)
}

// BlockInCycle reports whether control flow can return to start.
func BlockInCycle(start *ssa.BasicBlock) bool {
	return blockInCycleWithin(start, nil)
}

func blockInCycleWithin(start *ssa.BasicBlock, budget *SearchBudget) bool {
	// Starting at successors requires at least one edge. Starting at the block
	// itself would incorrectly classify every acyclic block as a cycle.
	return blockReachableFromWithin(start.Succs, start, budget)
}

// blockReachableFromWithin owns raw CFG traversal; callers choose whether the
// initial block or only its successors can count. Clone the seeds because
// queue growth must not overwrite an SSA block's successor backing array.
func blockReachableFromWithin(seeds []*ssa.BasicBlock, target *ssa.BasicBlock, budget *SearchBudget) bool {
	seen := map[*ssa.BasicBlock]bool{}
	queue := slices.Clone(seeds)
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
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
			queue = append(queue, successor)
		}
	}
	return false
}

// UnownedReturnQuery names one obligation for UnownedReturn: where it begins,
// what settles it, and what the caller may assume. Exactly one of After,
// AfterCallSuccess, and Entry is set.
type UnownedReturnQuery struct {
	// After starts the obligation just after this instruction.
	After ssa.Instruction
	// AfterCallSuccess starts it on the branch on which the call succeeded,
	// for obligations a successful call creates, such as exec.Cmd.Start: a
	// handled failure may rejoin a later return, but no obligation exists on
	// that path. Without a success branch it starts after the call.
	AfterCallSuccess *ssa.Call
	// Entry starts it at the function's entry.
	Entry *ssa.Function
	// Owns labels an instruction that settles the obligation exactly.
	Owns func(ssa.Instruction) bool
	// OwnsEdge labels a CFG edge that settles it; the action is attached to
	// that successor's state, never to sibling paths.
	OwnsEdge OwnershipEdge
	// AllowReturn marks a return that needs no settling action.
	AllowReturn func(*ssa.Return) bool
	// Assume restricts the walk to paths feasible under what the caller
	// knows: a non-nil value, such as a cleanup context.WithTimeout
	// guarantees even through an optional local, its concrete type, and
	// fixed Boolean parameters or captures.
	// https://github.com/agenticenv/agent-sdk-go/blob/63f0452159d674d529a6fea91b8d532bed9b774e/internal/runtime/local/agent_loop.go#L828-L841
	Assume EntryAssumptions
}

// OwnershipEdge describes an ownership action established only by taking a
// particular CFG edge, rather than by executing its branch instruction.
type OwnershipEdge func(from, to *ssa.BasicBlock) bool

// EntryAssumptions restricts a walk to the paths feasible under facts the
// caller knows: a non-nil value, its concrete type, so a comma-ok assertion
// of a type it satisfies is taken to succeed, and Boolean parameters or
// captures fixed by the call.
type EntryAssumptions struct {
	NonNil     ssa.Value
	NonNilType types.Type
	Constants  FixedValues
}

// UnownedReturn returns a normal return that some feasible path reaches from
// the query's start with no settling action before it, for a diagnostic to
// cite, or nil when every return is settled. It is the two-level view of the
// shared obligation walk: a settling action is exact coverage and everything
// else is none, so the only outcomes are honored and violated. Tracking the
// obligation through the CFG makes conditional cleanup visible without
// pretending infeasible branches are impossible.
func UnownedReturn(query UnownedReturnQuery) *ssa.Return {
	initial, ok := query.initialStates()
	if !ok {
		return nil
	}
	outcome, witness := obligationOutcome(initial, ObligationFlow{
		NonNil: query.Assume.NonNil, NonNilType: query.Assume.NonNilType, Constants: query.Assume.Constants,
		Instruction: ExactOrNone(query.Owns), Return: exactOrNoneReturn(query.AllowReturn), Edge: exactOrNoneEdge(query.OwnsEdge),
	})
	if outcome != ObligationViolated {
		return nil
	}
	return witness
}

// initialStates returns the walk's starting states for the query's start.
func (query UnownedReturnQuery) initialStates() ([]obligationState, bool) {
	switch {
	case query.Entry != nil:
		if len(query.Entry.Blocks) == 0 {
			return nil, false
		}
		return []obligationState{{block: query.Entry.Blocks[0]}}, true
	case query.AfterCallSuccess != nil:
		call := query.AfterCallSuccess
		for _, successor := range call.Block().Succs {
			if success, known := SuccessBranch(call.Block(), successor, call); known && success {
				return []obligationState{{block: successor, predecessor: call.Block()}}, true
			}
		}
		return afterInstruction(call)
	case query.After != nil:
		return afterInstruction(query.After)
	}
	return nil, false
}

func afterInstruction(start ssa.Instruction) ([]obligationState, bool) {
	index := InstructionIndex(start)
	if index < 0 {
		return nil, false
	}
	return []obligationState{{block: start.Block(), index: index + 1}}, true
}

// ReachableBlocksAssumingWithin returns blocks reachable under constants in
// discovery order, sharing queued, branch and edge visits with budget. Cutoff
// discards the census; nil is unavailable when budget exhausted,
// not proof that the function has no reachable blocks. Nil budget is unbounded.
func ReachableBlocksAssumingWithin(function *ssa.Function, constants FixedValues, budget *SearchBudget) []*ssa.BasicBlock {
	if function == nil || len(function.Blocks) == 0 {
		return nil
	}
	var order []*ssa.BasicBlock
	WalkStatesWithin([]*ssa.BasicBlock{function.Blocks[0]}, func(block *ssa.BasicBlock) *ssa.BasicBlock { return block },
		func(block *ssa.BasicBlock) ([]*ssa.BasicBlock, bool) {
			order = append(order, block)
			successors := constants.NarrowWithin(block.Succs, block, budget)
			for range successors {
				if !budget.Spend() {
					return nil, false
				}
			}
			return successors, true
		}, budget)
	if budget.Exhausted() {
		return nil
	}
	return order
}

// SuccessBranch reports whether successor is the branch where errorValue is
// nil, when block ends in a recognizable nil comparison.
func SuccessBranch(block, successor *ssa.BasicBlock, errorValue ssa.Value) (bool, bool) {
	if errorValue == nil || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return false, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return false, false
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false, false
	}
	comparesErrorToNil := derivesStructurally(comparison.X, errorValue) && DefinitelyNil(comparison.Y) ||
		derivesStructurally(comparison.Y, errorValue) && DefinitelyNil(comparison.X)
	if !comparesErrorToNil {
		return false, false
	}
	trueBranch := successor == block.Succs[0]
	if comparison.Op == token.EQL {
		return trueBranch, true
	}
	return !trueBranch, true
}
