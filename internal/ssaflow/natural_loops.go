package ssaflow

import (
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/ssa"
)

// NaturalLoop is a loop entered only through its header. Blocks holds the
// header and every block that reaches a back edge without passing through the
// header again, in function order; nested loops are part of the outer one.
// Exits are the blocks outside the loop that its blocks branch to, in the
// order they are first seen, so a break or a return inside the body is an
// exit like the header's own test.
type NaturalLoop struct {
	Header *ssa.BasicBlock
	Blocks []*ssa.BasicBlock
	Exits  []*ssa.BasicBlock
}

// Contains reports whether block belongs to the loop.
func (loop NaturalLoop) Contains(block *ssa.BasicBlock) bool {
	return slices.Contains(loop.Blocks, block)
}

// OutermostLoops returns the function's natural loops that no other loop
// contains. A cycle that is not a natural loop is not reported; a caller that
// orders blocks still finds it and declines.
func OutermostLoops(function *ssa.Function, budget *SearchBudget) ([]NaturalLoop, bool) {
	var loops []NaturalLoop
	for _, header := range function.Blocks {
		loop, ok := naturalLoop(header, budget)
		if budget.Exhausted() {
			return nil, false
		}
		if ok {
			loops = append(loops, loop)
		}
	}
	outermost := loops[:0:0]
	for _, loop := range loops {
		nested := slices.ContainsFunc(loops, func(other NaturalLoop) bool {
			return other.Header != loop.Header && other.Contains(loop.Header)
		})
		if !nested {
			outermost = append(outermost, loop)
		}
	}
	return outermost, true
}

// naturalLoop collects the loop whose back edges enter header: edges from
// blocks that header dominates.
func naturalLoop(header *ssa.BasicBlock, budget *SearchBudget) (NaturalLoop, bool) {
	members := map[*ssa.BasicBlock]bool{header: true}
	var work []*ssa.BasicBlock
	backEdge := false
	for _, predecessor := range header.Preds {
		if !header.Dominates(predecessor) {
			continue
		}
		// A one-block loop, such as a range over an integer, branches back
		// to itself.
		backEdge = true
		if !members[predecessor] {
			members[predecessor] = true
			work = append(work, predecessor)
		}
	}
	if !backEdge {
		return NaturalLoop{}, false
	}
	for len(work) != 0 {
		if !budget.Spend() {
			return NaturalLoop{}, false
		}
		block := work[len(work)-1]
		work = work[:len(work)-1]
		for _, predecessor := range block.Preds {
			if !members[predecessor] {
				members[predecessor] = true
				work = append(work, predecessor)
			}
		}
	}
	loop := NaturalLoop{Header: header}
	for _, block := range header.Parent().Blocks {
		if members[block] {
			loop.Blocks = append(loop.Blocks, block)
		}
	}
	for _, block := range loop.Blocks {
		for _, next := range block.Succs {
			if !members[next] && !slices.Contains(loop.Exits, next) {
				loop.Exits = append(loop.Exits, next)
			}
		}
	}
	return loop, true
}

// BoundedLoop reports whether every loop in loop, nested ones included, is
// driven by an integer counter that rises by one on each iteration toward a
// bound computed before the loop, so each ends after finitely many
// iterations. Range loops over slices, arrays, strings, and integers, and
// ordinary counted for loops, have this shape. It says nothing about whether
// the calls in the body return; the caller decides that separately.
func BoundedLoop(loop NaturalLoop, budget *SearchBudget) bool {
	for _, block := range loop.Blocks {
		if !budget.Spend() {
			return false
		}
		inner, ok := naturalLoop(block, budget)
		if ok && !boundedCounter(inner) {
			return false
		}
	}
	return !budget.Exhausted()
}

// boundedCounter finds the loop's exit test x < b. It must run on every
// iteration, so its block dominates every back edge, and its false edge must
// leave the loop. x is a header counter p or p+1, where every back edge
// supplies p+1, and b is defined outside the loop.
func boundedCounter(loop NaturalLoop) bool {
	for _, block := range loop.Blocks {
		test, ok := exitTest(loop, block)
		if !ok || !dominatesBackEdges(loop, block) {
			continue
		}
		counter := risingCounter(loop, test.X)
		if counter != nil && invariant(loop, test.Y) && integer(counter.Type()) {
			return true
		}
	}
	return false
}

func exitTest(loop NaturalLoop, block *ssa.BasicBlock) (*ssa.BinOp, bool) {
	if len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return nil, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok || !loop.Contains(block.Succs[0]) || loop.Contains(block.Succs[1]) {
		return nil, false
	}
	test, ok := branch.Cond.(*ssa.BinOp)
	return test, ok && test.Op == token.LSS
}

func dominatesBackEdges(loop NaturalLoop, block *ssa.BasicBlock) bool {
	for _, predecessor := range loop.Header.Preds {
		if loop.Contains(predecessor) && !block.Dominates(predecessor) {
			return false
		}
	}
	return true
}

// risingCounter returns the header phi p when value is p or p+1 and every
// back edge into the header supplies p+1.
func risingCounter(loop NaturalLoop, value ssa.Value) *ssa.Phi {
	counter, ok := value.(*ssa.Phi)
	if !ok {
		if step := unitStep(value); step != nil {
			counter, ok = step.X.(*ssa.Phi)
		}
	}
	if !ok || counter.Block() != loop.Header {
		return nil
	}
	for predecessor, incoming := range PhiIncoming(counter) {
		if !loop.Contains(predecessor) {
			continue
		}
		if step := unitStep(incoming); step == nil || step.X != counter {
			return nil
		}
	}
	return counter
}

func unitStep(value ssa.Value) *ssa.BinOp {
	step, ok := value.(*ssa.BinOp)
	if !ok || step.Op != token.ADD || !integerLiteral(step.Y, 1) {
		return nil
	}
	return step
}

// invariant reports whether value is the same on every iteration: defined
// outside the loop, or the length of a slice, array, or string that is. A
// channel's or map's length can change while the loop runs.
func invariant(loop NaturalLoop, value ssa.Value) bool {
	switch value := value.(type) {
	case *ssa.Const, *ssa.Parameter, *ssa.FreeVar:
		return true
	case *ssa.Call:
		if !loop.Contains(value.Block()) {
			return true
		}
		builtin, ok := value.Call.Value.(*ssa.Builtin)
		return ok && builtin.Name() == "len" && len(value.Call.Args) == 1 &&
			fixedLength(value.Call.Args[0].Type()) && invariant(loop, value.Call.Args[0])
	case ssa.Instruction:
		return !loop.Contains(value.Block())
	default:
		return false
	}
}

func fixedLength(value types.Type) bool {
	switch value := value.Underlying().(type) {
	case *types.Slice, *types.Array:
		return true
	case *types.Basic:
		return value.Info()&types.IsString != 0
	case *types.Pointer:
		_, array := value.Elem().Underlying().(*types.Array)
		return array
	default:
		return false
	}
}

func integer(value types.Type) bool {
	basic, ok := value.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}
