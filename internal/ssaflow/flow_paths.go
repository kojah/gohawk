package ssaflow

import (
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Control-flow evidence answers path-sensitive ownership questions shared by
// several analyzers. Traversal retains the predecessor for phi and branch
// feasibility, and treats only reachable normal returns as lifecycle exits.

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

// ReachableBlocksAssuming returns the blocks some path from entry reaches
// when the bound constants hold, in discovery order.
func ReachableBlocksAssuming(function *ssa.Function, constants FixedValues) []*ssa.BasicBlock {
	if function == nil || len(function.Blocks) == 0 {
		return nil
	}
	reached := map[*ssa.BasicBlock]bool{function.Blocks[0]: true}
	order := []*ssa.BasicBlock{function.Blocks[0]}
	for index := 0; index < len(order); index++ {
		block := order[index]
		for _, next := range constants.Narrow(block.Succs, block) {
			if !reached[next] {
				reached[next] = true
				order = append(order, next)
			}
		}
	}
	return order
}

// assumedSuccessors narrows already-feasible successors by the assumption
// that value is non-nil at the branch and, when its concrete type is
// known, that a comma-ok assertion of a type that concrete type satisfies
// succeeds.
func assumedSuccessors(successors []*ssa.BasicBlock, block *ssa.BasicBlock, value ssa.Value, concrete types.Type) []*ssa.BasicBlock {
	if value == nil || len(block.Succs) != 2 || len(block.Instrs) == 0 {
		return successors
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return successors
	}
	// kubernetes closes a writer through a helper that asserts io.Closer on
	// it; for a caller passing an *os.File the assertion holds, so the arm
	// without the close is not a path the file takes.
	// https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/test/e2e/storage/podlogs/podlogs.go#L360-L364
	if concrete != nil && assertionHolds(branch.Cond, value, concrete) {
		for _, successor := range successors {
			if successor == block.Succs[0] {
				return []*ssa.BasicBlock{successor}
			}
		}
		return nil
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

// assertionHolds reports whether condition is the ok result of a comma-ok
// assertion of the assumed value to a type its concrete type satisfies.
func assertionHolds(condition, value ssa.Value, concrete types.Type) bool {
	okResult, ok := condition.(*ssa.Extract)
	if !ok || okResult.Index != 1 {
		return false
	}
	assertion, ok := okResult.Tuple.(*ssa.TypeAssert)
	return ok && assertion.CommaOk && StructurallyIdentical(assertion.X, value) && types.AssignableTo(concrete, assertion.AssertedType)
}

// assumedNonNil reports whether operand is the assumed value itself or a
// field loaded directly from it.
func assumedNonNil(operand, value ssa.Value) bool {
	if StructurallyIdentical(operand, value) {
		return true
	}
	load, ok := operand.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	field, ok := load.X.(*ssa.FieldAddr)
	return ok && StructurallyIdentical(field.X, value)
}

// FeasibleSuccessors preserves constants selected by predecessor-sensitive
// phis and literal results of bounded, source-visible helpers. This prevents
// impossible loop exits and helper-error paths from faking leaks.
func FeasibleSuccessors(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock {
	if len(block.Succs) != 2 || len(block.Instrs) == 0 {
		return block.Succs
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return block.Succs
	}
	value, known := BranchBool(branch.Cond, block, predecessor)
	if !known {
		return block.Succs
	}
	if value {
		return block.Succs[:1]
	}
	return block.Succs[1:]
}

func BranchBool(value ssa.Value, block, predecessor *ssa.BasicBlock) (bool, bool) {
	if literal := branchLiteral(value, block, predecessor); literal != nil && literal.Value != nil && literal.Value.Kind() == constant.Bool {
		return constant.BoolVal(literal.Value), true
	}
	// Resolve the first condition of constant-count loops. Otherwise the flow
	// engine invents a zero-iteration path and can report workers as unjoined
	// even when a positive fixed-count receive loop follows them, as in:
	// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/internal/cri/store/stats/timed_store_test.go#L190-L222
	if comparison, ok := value.(*ssa.BinOp); ok {
		return compareBranchLiterals(comparison, block, predecessor)
	}
	phi, ok := value.(*ssa.Phi)
	if !ok || phi.Block() != block || predecessor == nil {
		return false, false
	}
	for index, candidate := range block.Preds {
		if candidate == predecessor && index < len(phi.Edges) {
			return BranchBool(phi.Edges[index], block, nil)
		}
	}
	return false, false
}

func compareBranchLiterals(comparison *ssa.BinOp, block, predecessor *ssa.BasicBlock) (bool, bool) {
	left := branchLiteral(comparison.X, block, predecessor)
	right := branchLiteral(comparison.Y, block, predecessor)
	if left == nil || right == nil {
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

func branchLiteral(value ssa.Value, block, predecessor *ssa.BasicBlock) *ssa.Const {
	if literal, ok := value.(*ssa.Const); ok {
		return literal
	}
	if literal := callResultLiteral(value); literal != nil {
		return literal
	}
	phi, ok := value.(*ssa.Phi)
	if !ok || phi.Block() != block || predecessor == nil {
		return nil
	}
	for index, candidate := range block.Preds {
		if candidate == predecessor && index < len(phi.Edges) {
			return branchLiteral(phi.Edges[index], block, nil)
		}
	}
	return nil
}

// A returned literal is independent of helper side effects and arguments.
// Inspect actual return operands, not a named result's initial zero value:
// deferred mutation, merged results, unavailable bodies, and recursion through
// returned calls remain opaque. No callee traversal or path enumeration occurs.
// https://github.com/raskrebs/sonar/blob/9c963b8447d6ca08dd4a3c0bc6c0bf27527cd793/internal/spawn/spawn_unix.go#L27
func callResultLiteral(value ssa.Value) *ssa.Const {
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
	budget := 128
	for _, block := range callee.Blocks {
		for _, instruction := range block.Instrs {
			budget--
			if budget < 0 {
				return nil
			}
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
	}
	return result
}

func sameLiteral(left, right *ssa.Const) bool {
	if left.IsNil() || right.IsNil() {
		return left.IsNil() && right.IsNil()
	}
	return left.Value != nil && right.Value != nil && constant.Compare(left.Value, token.EQL, right.Value)
}

// NormalReturnReachableFrom reports whether block can reach a normal return
// without first invoking a control-flow terminating API.
func NormalReturnReachableFrom(block *ssa.BasicBlock) bool {
	return NormalReturnReachableWith(block, nil)
}

// NormalReturnReachableWith is NormalReturnReachableFrom with the catalog of
// terminating calls extended by a terminator.
func NormalReturnReachableWith(block *ssa.BasicBlock, terminates Terminator) bool {
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
			if InstructionTerminatesWith(instruction, terminates) {
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
