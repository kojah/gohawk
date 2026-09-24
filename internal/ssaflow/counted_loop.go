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
	if !budget.Spend() {
		return CountedLoop{Reason: LoopBudgetExhausted}
	}
	counter, comparison := countedHeader(header)
	if counter == nil {
		return CountedLoop{Reason: LoopShapeUnknown}
	}
	bound, literal := comparison.Y.(*ssa.Const)
	if !literal || bound.Value == nil || bound.Value.Kind() != constant.Int {
		return CountedLoop{Reason: LoopShapeUnknown}
	}
	n, exact := constant.Int64Val(bound.Value)
	if !exact || n < 0 || n > int64(limit) {
		return CountedLoop{Reason: LoopCountOverLimit}
	}
	body := header.Succs[0]
	if len(body.Preds) != 1 || len(body.Succs) != 1 || body.Succs[0] != header {
		return CountedLoop{Reason: LoopShapeUnknown}
	}
	step := inductionStep(counter, body)
	if step == nil {
		return CountedLoop{Reason: LoopShapeUnknown}
	}
	used := !usesOnly(counter, budget, step, comparison) || !usesOnly(step, budget, counter)
	if budget.Exhausted() {
		return CountedLoop{Reason: LoopBudgetExhausted}
	}
	return CountedLoop{Body: body, Exit: header.Succs[1], Count: int(n), CounterUsed: used, Reason: LoopCountKnown}
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

func inductionStep(counter *ssa.Phi, body *ssa.BasicBlock) *ssa.BinOp {
	var step *ssa.BinOp
	for predecessor, incoming := range PhiIncoming(counter) {
		if predecessor == body {
			step, _ = incoming.(*ssa.BinOp)
		} else if !integerLiteral(incoming, 0) {
			return nil
		}
	}
	if step == nil || step.Block() != body || step.Op != token.ADD || step.X != counter || !integerLiteral(step.Y, 1) {
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
