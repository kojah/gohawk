package ssaflow

import (
	"go/constant"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// Control-flow evidence answers path-sensitive ownership questions shared by
// several analyzers. Traversal retains the predecessor for phi and branch
// feasibility, and treats only reachable normal returns as lifecycle exits.

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

// InstructionDominates reports whether every path to after executes before.
// Instruction order is respected when both values belong to one block.
func InstructionDominates(before, after ssa.Instruction) bool {
	if before == nil || after == nil || before.Parent() != after.Parent() {
		return false
	}
	if before.Block() == after.Block() {
		return InstructionIndex(before) <= InstructionIndex(after)
	}
	return before.Block().Dominates(after.Block())
}

// InstructionMayFollow reports whether after is reachable after before. This
// is intentionally weaker than dominance and is used only to reject evidence
// that is earlier than, or disconnected from, the obligation it purports to
// settle.
func InstructionMayFollow(before, after ssa.Instruction) bool {
	if before == nil || after == nil || before.Parent() != after.Parent() {
		return false
	}
	if before.Block() == after.Block() {
		return InstructionIndex(before) <= InstructionIndex(after)
	}
	seen := map[*ssa.BasicBlock]bool{}
	queue := append([]*ssa.BasicBlock(nil), before.Block().Succs...)
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == after.Block() {
			return true
		}
		if seen[block] {
			continue
		}
		seen[block] = true
		queue = append(queue, block.Succs...)
	}
	return false
}

// BlockReachable reports whether target is reachable from within their
// shared function.
func BlockReachable(from, target *ssa.BasicBlock) bool {
	if from == nil || target == nil || from.Parent() != target.Parent() {
		return false
	}
	seen := map[*ssa.BasicBlock]bool{}
	queue := []*ssa.BasicBlock{from}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == target {
			return true
		}
		if seen[block] {
			continue
		}
		seen[block] = true
		queue = append(queue, block.Succs...)
	}
	return false
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
	return unownedReturnFrom([]flowState{{block: start.Block(), index: index + 1}}, owns, allowReturn)
}

// UnownedReturnAfterCallSuccess is UnownedReturn restricted to the branch on
// which call succeeded. This matters for obligations created by successful
// calls such as exec.Cmd.Start: a handled failure may rejoin a later return,
// but no ownership obligation exists on that path.
func UnownedReturnAfterCallSuccess(
	call *ssa.Call,
	owns func(ssa.Instruction) bool,
	allowReturn func(*ssa.Return) bool,
) bool {
	if call == nil {
		return false
	}
	for _, successor := range call.Block().Succs {
		if success, known := SuccessBranch(call.Block(), successor, call); known && success {
			return unownedReturnFrom([]flowState{{block: successor, predecessor: call.Block()}}, owns, allowReturn)
		}
	}
	return UnownedReturn(call, owns, allowReturn)
}

func unownedReturnFrom(
	queue []flowState,
	owns func(ssa.Instruction) bool,
	allowReturn func(*ssa.Return) bool,
) bool {
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
		terminated := false
		for _, instruction := range state.block.Instrs[state.index:] {
			state.owned = state.owned || owns(instruction)
			if InstructionTerminatesControlFlow(instruction) {
				terminated = true
				break
			}
			returned, ok := instruction.(*ssa.Return)
			if ok && !state.owned && (allowReturn == nil || !allowReturn(returned)) {
				return true
			}
		}
		if terminated {
			continue
		}
		for _, successor := range FeasibleSuccessors(state.block, state.predecessor) {
			queue = append(queue, flowState{block: successor, predecessor: state.block, owned: state.owned})
		}
	}
	return false
}

// UnownedReturnAssumingNonNil is UnownedReturn with the additional fact that
// value is non-nil after start. Constructors such as context.WithTimeout
// guarantee a callable cleanup even when it flows through an optional local.
// https://github.com/agenticenv/agent-sdk-go/blob/63f0452159d674d529a6fea91b8d532bed9b774e/internal/runtime/local/agent_loop.go#L828-L841
func UnownedReturnAssumingNonNil(
	start ssa.Instruction,
	value ssa.Value,
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
		terminated := false
		for _, instruction := range state.block.Instrs[state.index:] {
			state.owned = state.owned || owns(instruction)
			if InstructionTerminatesControlFlow(instruction) {
				terminated = true
				break
			}
			if returned, ok := instruction.(*ssa.Return); ok && !state.owned && (allowReturn == nil || !allowReturn(returned)) {
				return true
			}
		}
		if terminated {
			continue
		}
		for _, successor := range nonNilFeasibleSuccessors(state.block, state.predecessor, value) {
			queue = append(queue, flowState{block: successor, predecessor: state.block, owned: state.owned})
		}
	}
	return false
}

// UnownedReturnFromEntry reports whether any normal return lacks an ownership action.
func UnownedReturnFromEntry(function *ssa.Function, owns func(ssa.Instruction) bool) bool {
	return unownedReturnFromEntry(function, owns, nil, nil)
}

// UnownedReturnFromEntryAllow reports whether any normal return lacks an
// ownership action unless allowReturn proves that return needs none.
func UnownedReturnFromEntryAllow(function *ssa.Function, owns func(ssa.Instruction) bool, allowReturn func(*ssa.Return) bool) bool {
	return unownedReturnFromEntry(function, owns, allowReturn, nil)
}

// UnownedReturnFromEntryAssumingNonNil analyzes only paths feasible when value
// is non-nil at function entry.
func UnownedReturnFromEntryAssumingNonNil(function *ssa.Function, value ssa.Value, owns func(ssa.Instruction) bool) bool {
	return unownedReturnFromEntry(function, owns, nil, value)
}

func unownedReturnFromEntry(function *ssa.Function, owns func(ssa.Instruction) bool, allowReturn func(*ssa.Return) bool, nonNil ssa.Value) bool {
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
		terminated := false
		for _, instruction := range state.block.Instrs {
			state.owned = state.owned || owns(instruction)
			if InstructionTerminatesControlFlow(instruction) {
				terminated = true
				break
			}
			if returned, ok := instruction.(*ssa.Return); ok && !state.owned && (allowReturn == nil || !allowReturn(returned)) {
				return true
			}
		}
		if terminated {
			continue
		}
		for _, successor := range nonNilFeasibleSuccessors(state.block, state.predecessor, nonNil) {
			queue = append(queue, flowState{block: successor, predecessor: state.block, owned: state.owned})
		}
	}
	return false
}

func nonNilFeasibleSuccessors(block, predecessor *ssa.BasicBlock, value ssa.Value) []*ssa.BasicBlock {
	successors := FeasibleSuccessors(block, predecessor)
	if value == nil || len(block.Succs) != 2 || len(block.Instrs) == 0 {
		return successors
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return successors
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return successors
	}
	// Use identity, not general data derivation: an error returned by a call
	// that received the context may derive from the constructor result without
	// being the cleanup function whose non-nilness is known. A nil-comparable
	// field loaded from the value, such as resp.Body, is assumed non-nil with
	// it: when the field is nil there is nothing to release, so the branch
	// that skips the release on that account proves no leak. autobrr's shared
	// drain helper guards on both:
	// https://github.com/autobrr/autobrr/blob/31a08a55a4539d846f1c68bfef43798659e05596/pkg/sharedhttp/http.go#L109-L114
	comparesNil := assumedNonNil(comparison.X, value) && DefinitelyNil(comparison.Y) ||
		assumedNonNil(comparison.Y, value) && DefinitelyNil(comparison.X)
	if !comparesNil {
		return successors
	}
	nonNil := block.Succs[0]
	if comparison.Op == token.EQL {
		nonNil = block.Succs[1]
	}
	for _, successor := range successors {
		if successor == nonNil {
			return []*ssa.BasicBlock{successor}
		}
	}
	return nil
}

// assumedNonNil reports whether operand is the assumed value itself or a
// field loaded directly from it.
func assumedNonNil(operand, value ssa.Value) bool {
	if SameValue(operand, value) {
		return true
	}
	load, ok := operand.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	field, ok := load.X.(*ssa.FieldAddr)
	return ok && SameValue(field.X, value)
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
			default:
				return false, false
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

// NormalReturnReachableFrom reports whether block can reach a normal return
// without first invoking a control-flow terminating API.
func NormalReturnReachableFrom(block *ssa.BasicBlock) bool {
	queue := []*ssa.BasicBlock{block}
	seen := map[*ssa.BasicBlock]bool{}
	for len(queue) > 0 {
		candidate := queue[0]
		queue = queue[1:]
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		terminated := false
		for _, instruction := range candidate.Instrs {
			if InstructionTerminatesControlFlow(instruction) {
				terminated = true
				break
			}
			if _, ok := instruction.(*ssa.Return); ok {
				return true
			}
		}
		if !terminated {
			queue = append(queue, candidate.Succs...)
		}
	}
	return false
}
