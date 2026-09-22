package ssaflow

import (
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// SelectedReceiveChannel returns the channel necessarily received from before
// entering block. The predecessor must select that exact receive case through
// SSA's select index; a default, send case, or shared successor proves nothing.
// This only describes the selected operation, not its synchronization policy.
func SelectedReceiveChannel(block *ssa.BasicBlock) (ssa.Value, bool) { //nolint:ireturn // Channels retain concrete SSA identities.
	if block == nil || len(block.Preds) != 1 {
		return nil, false
	}
	return SelectedReceiveOnEdge(block.Preds[0], block)
}

// SelectedReceiveOnEdge returns the channel received from when the exact
// select-case edge is taken. Other predecessors of to may establish no receive;
// callers must keep this evidence on the edge, not on the shared destination.
func SelectedReceiveOnEdge(from, to *ssa.BasicBlock) (ssa.Value, bool) { //nolint:ireturn // Channels retain concrete SSA identities.
	if from == nil || to == nil || len(from.Instrs) == 0 || len(from.Succs) != 2 || from.Succs[0] != to {
		return nil, false
	}
	branch, ok := from.Instrs[len(from.Instrs)-1].(*ssa.If)
	if !ok {
		return nil, false
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL {
		return nil, false
	}
	index, ok := comparison.X.(*ssa.Extract)
	arm, constantArm := comparison.Y.(*ssa.Const)
	if !ok || index.Index != 0 || !constantArm || arm.Value == nil {
		return nil, false
	}
	selected, ok := index.Tuple.(*ssa.Select)
	if !ok || arm.Value.Kind() != constant.Int {
		return nil, false
	}
	number, valid := constant.Int64Val(arm.Value)
	if !valid || number < 0 || number >= int64(len(selected.States)) {
		return nil, false
	}
	state := selected.States[number]
	return state.Chan, state.Dir == types.RecvOnly
}
