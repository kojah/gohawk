package ssautil

import (
	"go/constant"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

type flowState struct {
	block       *ssa.BasicBlock
	predecessor *ssa.BasicBlock
	index       int
	owned       bool
}

type flowKey struct {
	block       int
	predecessor int
	index       int
	owned       bool
}

// InstructionIndex returns instruction position within its basic block.
func InstructionIndex(instruction ssa.Instruction) int {
	for index, candidate := range instruction.Block().Instrs {
		if candidate == instruction {
			return index
		}
	}
	return -1
}

// UnownedReturn reports whether any normal return reachable after start lacks
// an ownership action. Tracking owned state through CFG makes conditional
// cleanup visible without pretending infeasible branches are impossible.
func UnownedReturn(
	start ssa.Instruction,
	owns func(ssa.Instruction) bool,
	allowReturn func(*ssa.Return) bool,
) bool {
	index := InstructionIndex(start)
	if index < 0 {
		return false
	}
	queue := []flowState{{block: start.Block(), index: index + 1}}
	seen := map[flowKey]bool{}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		predecessor := -1
		if state.predecessor != nil {
			predecessor = state.predecessor.Index
		}
		key := flowKey{block: state.block.Index, predecessor: predecessor, index: state.index, owned: state.owned}
		if seen[key] {
			continue
		}
		seen[key] = true
		for _, instruction := range state.block.Instrs[state.index:] {
			state.owned = state.owned || owns(instruction)
			returned, ok := instruction.(*ssa.Return)
			if ok && !state.owned && (allowReturn == nil || !allowReturn(returned)) {
				return true
			}
		}
		for _, successor := range FeasibleSuccessors(state.block, state.predecessor) {
			queue = append(queue, flowState{block: successor, predecessor: state.block, owned: state.owned})
		}
	}
	return false
}

// UnownedReturnFromEntry reports whether any normal return lacks an ownership action.
func UnownedReturnFromEntry(function *ssa.Function, owns func(ssa.Instruction) bool) bool {
	if len(function.Blocks) == 0 {
		return false
	}
	queue := []flowState{{block: function.Blocks[0]}}
	seen := map[flowKey]bool{}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		predecessor := -1
		if state.predecessor != nil {
			predecessor = state.predecessor.Index
		}
		key := flowKey{block: state.block.Index, predecessor: predecessor, owned: state.owned}
		if seen[key] {
			continue
		}
		seen[key] = true
		for _, instruction := range state.block.Instrs {
			state.owned = state.owned || owns(instruction)
			if _, ok := instruction.(*ssa.Return); ok && !state.owned {
				return true
			}
		}
		for _, successor := range FeasibleSuccessors(state.block, state.predecessor) {
			queue = append(queue, flowState{block: successor, predecessor: state.block, owned: state.owned})
		}
	}
	return false
}

// FeasibleSuccessors preserves constants selected by predecessor-sensitive
// phis. This prevents impossible first-iteration loop exits from faking leaks.
func FeasibleSuccessors(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock {
	if len(block.Succs) != 2 || len(block.Instrs) == 0 {
		return block.Succs
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return block.Succs
	}
	value, known := branchBool(branch.Cond, block, predecessor)
	if !known {
		return block.Succs
	}
	if value {
		return block.Succs[:1]
	}
	return block.Succs[1:]
}

func branchBool(value ssa.Value, block, predecessor *ssa.BasicBlock) (bool, bool) {
	if literal, ok := value.(*ssa.Const); ok && literal.Value != nil && literal.Value.Kind() == constant.Bool {
		return constant.BoolVal(literal.Value), true
	}
	// Resolve the first condition of constant-count loops. Otherwise the flow
	// engine invents a zero-iteration path and can report workers as unjoined
	// even when a positive fixed-count receive loop follows them, as in:
	// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/internal/cri/store/stats/timed_store_test.go#L190-L222
	if comparison, ok := value.(*ssa.BinOp); ok {
		left, leftOK := branchConstant(comparison.X, block, predecessor)
		right, rightOK := branchConstant(comparison.Y, block, predecessor)
		if leftOK && rightOK {
			switch comparison.Op {
			case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
				return constant.Compare(left, comparison.Op, right), true
			}
		}
	}
	phi, ok := value.(*ssa.Phi)
	if !ok || phi.Block() != block || predecessor == nil {
		return false, false
	}
	for index, candidate := range block.Preds {
		if candidate == predecessor && index < len(phi.Edges) {
			return branchBool(phi.Edges[index], block, nil)
		}
	}
	return false, false
}

func branchConstant(value ssa.Value, block, predecessor *ssa.BasicBlock) (constant.Value, bool) {
	if literal, ok := value.(*ssa.Const); ok && literal.Value != nil {
		return literal.Value, true
	}
	phi, ok := value.(*ssa.Phi)
	if !ok || phi.Block() != block || predecessor == nil {
		return nil, false
	}
	for index, candidate := range block.Preds {
		if candidate == predecessor && index < len(phi.Edges) {
			return branchConstant(phi.Edges[index], block, nil)
		}
	}
	return nil, false
}

// ReachableReturns returns normal returns reachable after start.
func ReachableReturns(start ssa.Instruction) []*ssa.Return {
	index := InstructionIndex(start)
	if index < 0 {
		return nil
	}
	queue := []flowState{{block: start.Block(), index: index + 1}}
	seen := map[flowKey]bool{}
	var returns []*ssa.Return
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		key := flowKey{block: state.block.Index, index: state.index}
		if seen[key] {
			continue
		}
		seen[key] = true
		for _, instruction := range state.block.Instrs[state.index:] {
			if returned, ok := instruction.(*ssa.Return); ok {
				returns = append(returns, returned)
			}
		}
		for _, successor := range state.block.Succs {
			queue = append(queue, flowState{block: successor})
		}
	}
	return returns
}
