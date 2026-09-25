package ssaflow

import (
	"go/constant"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// CountedLoopReason distinguishes an exact control-flow count from unsupported
// shape, a caller-selected expansion limit, and exhausted analysis work.
type CountedLoopReason uint8

const (
	LoopShapeUnknown CountedLoopReason = iota
	LoopCountKnown
	LoopCountOverLimit
	LoopBudgetExhausted
)

// CountedLoop describes a zero-based unit-step loop with one body block.
// Count is the number of body entries if each iteration reaches its backedge,
// not a guarantee that calls inside the body terminate or return normally.
// CounterUsed distinguishes bodies requiring induction-value substitution
// from those whose ordinary instructions can be replayed unchanged.
type CountedLoop struct {
	Body, Exit  *ssa.BasicBlock
	Count       int
	CounterUsed bool
	Reason      CountedLoopReason
}

// Proven reports whether the header establishes an exact bounded count.
func (loop CountedLoop) Proven() bool { return loop.Reason == LoopCountKnown }

// ProveCountedLoop recognizes only i := 0; i < literal; i++ with one body
// block and no alternate entry or exit. The consumer supplies its expansion
// limit and decides whether body effects and iteration-local objects are safe
// to repeat. This query does not unroll SSA or choose an analysis policy.
func ProveCountedLoop(header *ssa.BasicBlock, limit int, budget *SearchBudget) CountedLoop {
	loop, counter, comparison := countedBound(header, limit, budget)
	if loop.Reason != LoopCountKnown {
		return loop
	}
	body := header.Succs[0]
	if len(body.Preds) != 1 || len(body.Succs) != 1 || body.Succs[0] != header {
		return CountedLoop{Reason: LoopShapeUnknown}
	}
	return finishCountedLoop(loop, counter, comparison, body, budget)
}

// ProveCountedRegion recognizes the same counted header, but lets the body
// branch and rejoin. Every iteration enters Body exactly once, and the body
// may leave only by returning to the header through the increment or by
// panicking: a break, return, or goto out of the loop is unsupported. Count is
// therefore the number of times Body ran on any path that takes Exit. Unlike
// ProveCountedLoop, the body cannot be replayed as straight-line code, so a
// consumer may use the count but must reason about the body's paths itself.
func ProveCountedRegion(header *ssa.BasicBlock, limit int, budget *SearchBudget) CountedLoop {
	loop, counter, comparison := countedBound(header, limit, budget)
	if loop.Reason != LoopCountKnown {
		return loop
	}
	body := header.Succs[0]
	latch := countedLatch(counter)
	if len(body.Preds) != 1 || latch == nil || !leavesOnlyThroughHeader(header, body, budget) {
		if budget.Exhausted() {
			return CountedLoop{Reason: LoopBudgetExhausted}
		}
		return CountedLoop{Reason: LoopShapeUnknown}
	}
	return finishCountedLoop(loop, counter, comparison, latch, budget)
}

// countedBound reads the header's literal bound. The returned loop carries
// only the count and reason; the caller checks the body shape it supports.
func countedBound(header *ssa.BasicBlock, limit int, budget *SearchBudget) (CountedLoop, *ssa.Phi, *ssa.BinOp) {
	if !budget.Spend() {
		return CountedLoop{Reason: LoopBudgetExhausted}, nil, nil
	}
	counter, comparison := countedHeader(header)
	if counter == nil {
		return CountedLoop{Reason: LoopShapeUnknown}, nil, nil
	}
	bound, literal := comparison.Y.(*ssa.Const)
	if !literal || bound.Value == nil || bound.Value.Kind() != constant.Int {
		return CountedLoop{Reason: LoopShapeUnknown}, nil, nil
	}
	n, exact := constant.Int64Val(bound.Value)
	if !exact || n < 0 || n > int64(limit) {
		return CountedLoop{Reason: LoopCountOverLimit}, nil, nil
	}
	return CountedLoop{Body: header.Succs[0], Exit: header.Succs[1], Count: int(n), Reason: LoopCountKnown}, counter, comparison
}

func finishCountedLoop(loop CountedLoop, counter *ssa.Phi, comparison *ssa.BinOp, latch *ssa.BasicBlock, budget *SearchBudget) CountedLoop {
	step := inductionStep(counter, latch)
	if step == nil {
		return CountedLoop{Reason: LoopShapeUnknown}
	}
	loop.CounterUsed = !usesOnly(counter, budget, step, comparison) || !usesOnly(step, budget, counter)
	if budget.Exhausted() {
		return CountedLoop{Reason: LoopBudgetExhausted}
	}
	return loop
}

// countedLatch returns the header predecessor that supplies the next counter
// value; countedHeader already requires exactly two predecessors.
func countedLatch(counter *ssa.Phi) *ssa.BasicBlock {
	for predecessor, incoming := range PhiIncoming(counter) {
		if !integerLiteral(incoming, 0) {
			return predecessor
		}
	}
	return nil
}

// leavesOnlyThroughHeader collects the blocks reachable from body without
// passing the header. The loop is closed when none of them returns or falls
// through to Exit, and nothing outside enters them except through body. A
// panic ends the path without taking Exit, so it is allowed.
func leavesOnlyThroughHeader(header, body *ssa.BasicBlock, budget *SearchBudget) bool {
	region := map[*ssa.BasicBlock]bool{body: true}
	work := []*ssa.BasicBlock{body}
	for len(work) > 0 {
		block := work[len(work)-1]
		work = work[:len(work)-1]
		if !budget.Spend() || block == header.Succs[1] || (len(block.Succs) == 0 && !endsInPanic(block)) {
			return false
		}
		for _, successor := range block.Succs {
			if successor != header && !region[successor] {
				region[successor] = true
				work = append(work, successor)
			}
		}
	}
	for block := range region {
		for _, predecessor := range block.Preds {
			if predecessor != header && !region[predecessor] {
				return false
			}
		}
	}
	return true
}

func endsInPanic(block *ssa.BasicBlock) bool {
	if len(block.Instrs) == 0 {
		return false
	}
	_, panics := block.Instrs[len(block.Instrs)-1].(*ssa.Panic)
	return panics
}

func countedHeader(header *ssa.BasicBlock) (*ssa.Phi, *ssa.BinOp) {
	if header == nil || len(header.Instrs) != 3 || len(header.Preds) != 2 || len(header.Succs) != 2 {
		return nil, nil
	}
	counter, phi := header.Instrs[0].(*ssa.Phi)
	comparison, binary := header.Instrs[1].(*ssa.BinOp)
	branch, conditional := header.Instrs[2].(*ssa.If)
	if !phi || !binary || !conditional || comparison.Op != token.LSS || comparison.X != counter || branch.Cond != comparison {
		return nil, nil
	}
	return counter, comparison
}

func inductionStep(counter *ssa.Phi, latch *ssa.BasicBlock) *ssa.BinOp {
	var step *ssa.BinOp
	for predecessor, incoming := range PhiIncoming(counter) {
		if predecessor == latch {
			step, _ = incoming.(*ssa.BinOp)
		} else if !integerLiteral(incoming, 0) {
			return nil
		}
	}
	if step == nil || step.Block() != latch || step.Op != token.ADD || step.X != counter || !integerLiteral(step.Y, 1) {
		return nil
	}
	return step
}

func usesOnly(value ssa.Value, budget *SearchBudget, allowed ...ssa.Instruction) bool {
	if value.Referrers() == nil {
		return false
	}
	for _, use := range *value.Referrers() {
		if !budget.Spend() {
			return false
		}
		found := false
		for _, instruction := range allowed {
			found = found || use == instruction
		}
		if !found {
			return false
		}
	}
	return true
}

func integerLiteral(value ssa.Value, want int64) bool {
	literal, ok := value.(*ssa.Const)
	return ok && literal.Value != nil && literal.Value.Kind() == constant.Int &&
		constant.Compare(literal.Value, token.EQL, constant.MakeInt64(want))
}
