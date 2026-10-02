package ssaflow

import (
	"go/constant"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// Literal branch evidence selects only the incoming phi of the current path
// and agreeing literal helper returns. It does not infer helper effects or
// enumerate callee paths. Caller exhaustion never supplies pruning evidence;
// the independent helper census cap retains its existing undecided policy.

// FeasibleSuccessorsWithin shares allowance through incoming phi selection
// and literal helper return inspection. Cutoff keeps all successors; callers
// retain availability before judging paths. A nil budget preserves defaults.
func FeasibleSuccessorsWithin(block, predecessor *ssa.BasicBlock, budget *SearchBudget) []*ssa.BasicBlock {
	if len(block.Succs) != 2 || len(block.Instrs) == 0 {
		return block.Succs
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return block.Succs
	}
	value, known := BranchBoolWithin(branch.Cond, block, predecessor, budget)
	if !known {
		return block.Succs
	}
	if value {
		return block.Succs[:1]
	}
	return block.Succs[1:]
}

// BranchValue selects a phi's incoming value only when it belongs to block
// and predecessor identifies the edge the current path took into that block.
// Other values, missing predecessors, and phis from earlier blocks are returned
// unchanged. It neither enumerates alternatives nor infers their truth values.
func BranchValue(value ssa.Value, block, predecessor *ssa.BasicBlock) ssa.Value {
	return BranchValueWithin(value, block, predecessor, nil)
}

// BranchValueWithin charges value and incoming-edge visits before selecting
// an operand. Nil at cutoff is unavailable; callers inspect the budget.
func BranchValueWithin(value ssa.Value, block, predecessor *ssa.BasicBlock, budget *SearchBudget) ssa.Value {
	if !budget.Spend() {
		return nil
	}
	phi, ok := value.(*ssa.Phi)
	if !ok || phi.Block() != block || predecessor == nil {
		return value
	}
	for incoming, operand := range PhiIncoming(phi) {
		if !budget.Spend() {
			return nil
		}
		if incoming == predecessor {
			return operand
		}
	}
	return value
}

// BranchBoolWithin decides the existing literal-only branch policy under a
// shared allowance. Exhaustion supplies no decided truth value. A nil budget
// retains the default policy.
func BranchBoolWithin(value ssa.Value, block, predecessor *ssa.BasicBlock, budget *SearchBudget) (bool, bool) {
	incoming := BranchValueWithin(value, block, predecessor, budget)
	if budget.Exhausted() {
		return false, false
	}
	if incoming != value {
		value, predecessor = incoming, nil
	}
	literal := branchLiteralWithin(value, block, predecessor, budget)
	if budget.Exhausted() {
		return false, false
	}
	if literal != nil && literal.Value != nil && literal.Value.Kind() == constant.Bool {
		return constant.BoolVal(literal.Value), true
	}
	// Resolve the first condition of constant-count loops. Otherwise the flow
	// engine invents a zero-iteration path and can report workers as unjoined
	// even when a positive fixed-count receive loop follows them, as in:
	// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/internal/cri/store/stats/timed_store_test.go#L190-L222
	if comparison, ok := value.(*ssa.BinOp); ok {
		return compareBranchLiteralsWithin(comparison, block, predecessor, budget)
	}
	return false, false
}

func compareBranchLiteralsWithin(comparison *ssa.BinOp, block, predecessor *ssa.BasicBlock, budget *SearchBudget) (bool, bool) {
	left := branchLiteralWithin(comparison.X, block, predecessor, budget)
	if budget.Exhausted() {
		return false, false
	}
	right := branchLiteralWithin(comparison.Y, block, predecessor, budget)
	if budget.Exhausted() || left == nil || right == nil {
		return false, false
	}
	if left.IsNil() && right.IsNil() {
		return comparison.Op == token.EQL, comparison.Op == token.EQL || comparison.Op == token.NEQ
	}
	if left.Value == nil || right.Value == nil {
		return false, false
	}
	switch comparison.Op {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
		return constant.Compare(left.Value, comparison.Op, right.Value), true
	default:
		return false, false
	}
}

func branchLiteralWithin(value ssa.Value, block, predecessor *ssa.BasicBlock, budget *SearchBudget) *ssa.Const {
	value = BranchValueWithin(value, block, predecessor, budget)
	if budget.Exhausted() {
		return nil
	}
	if literal, ok := value.(*ssa.Const); ok {
		return literal
	}
	if literal := callResultLiteralWithin(value, budget); literal != nil {
		return literal
	}
	return nil
}

// A returned literal is independent of helper side effects and arguments.
// Inspect actual return operands, not a named result's initial zero value:
// deferred mutation, merged results, unavailable bodies, and recursion through
// returned calls remain opaque. No callee traversal or path enumeration occurs.
// https://github.com/raskrebs/sonar/blob/9c963b8447d6ca08dd4a3c0bc6c0bf27527cd793/internal/spawn/spawn_unix.go#L27
func callResultLiteralWithin(value ssa.Value, budget *SearchBudget) *ssa.Const {
	index := 0
	if result, ok := value.(*ssa.Extract); ok {
		value, index = result.Tuple, result.Index
	}
	call, ok := value.(*ssa.Call)
	if !ok {
		return nil
	}
	callee := call.Common().StaticCallee()
	if callee == nil || len(callee.Blocks) == 0 {
		return nil
	}
	var result *ssa.Const
	remaining := literalHelperInstructionLimit
	for instruction := range InstructionsWithin(callee, budget) {
		if remaining == 0 {
			return nil
		}
		remaining--
		returned, ok := instruction.(*ssa.Return)
		if !ok {
			continue
		}
		if index >= len(returned.Results) {
			return nil
		}
		literal, ok := returned.Results[index].(*ssa.Const)
		if !ok || result != nil && !sameLiteral(result, literal) {
			return nil
		}
		result = literal
	}
	if budget.Exhausted() {
		return nil
	}
	return result
}

// Preserve the existing helper-local cap independently of caller exhaustion:
// a larger helper supplies no literal guarantee but does not exhaust the flow.
const literalHelperInstructionLimit = 128

func sameLiteral(left, right *ssa.Const) bool {
	if left.IsNil() || right.IsNil() {
		return left.IsNil() && right.IsNil()
	}
	return left.Value != nil && right.Value != nil && constant.Compare(left.Value, token.EQL, right.Value)
}
