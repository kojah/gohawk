package ssaflow

import (
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// WrittenOnceCell returns the value stored in cell when that store is the
// cell's only write anywhere: the function only reads the cell, and every
// closure that captures it, nested ones included, only reads it too. Every
// read then yields that value, in any goroutine and at any time after the
// store, which is how a variable captured by several goroutines names one
// object. It answers identity only; whether the value itself is stable is the
// caller's question.
func WrittenOnceCell(cell *ssa.Alloc) (ssa.Value, bool) {
	return WrittenOnceCellWithin(cell, nil)
}

// WrittenOnceCellWithin applies the same once-stored identity contract under
// budget, including nested lexical captures. Cutoff discards the stored value;
// callers inspect the budget before interpreting rejection as complete.
func WrittenOnceCellWithin(cell *ssa.Alloc, budget *SearchBudget) (ssa.Value, bool) {
	store, ok := writtenOnceStoreWithin(cell, budget)
	if !ok {
		return nil, false
	}
	return store.Val, true
}

// WrittenOnceCellAtWithin returns the unique stored value only when its store
// dominates observation. This proves initialization at the caller-selected
// instruction, not a universal capture or invocation policy. The census and
// ordering share budget; callers retain availability before using rejection.
func WrittenOnceCellAtWithin(cell *ssa.Alloc, observation ssa.Instruction, budget *SearchBudget) (ssa.Value, bool) {
	store, ok := writtenOnceStoreWithin(cell, budget)
	if !ok || !InstructionDominatesWithin(store, observation, budget) {
		return nil, false
	}
	return store.Val, true
}

// Keep the exact store with the once-written identity evidence so consumers
// that need execution order can apply their own observation boundary. The
// identity-only API does not promise that a read occurs after this store.
func writtenOnceStoreWithin(cell *ssa.Alloc, budget *SearchBudget) (*ssa.Store, bool) {
	if !budget.Spend() || cell.Referrers() == nil {
		return nil, false
	}
	var stored *ssa.Store
	for use := range ReferrersWithin(cell, budget) {
		switch use := use.(type) {
		case *ssa.Store:
			if use.Addr != cell || stored != nil {
				return nil, false
			}
			stored = use
		case *ssa.UnOp:
			if use.Op != token.MUL {
				return nil, false
			}
		case *ssa.MakeClosure:
			if !capturedReadOnlyWithin(use, cell, budget) {
				return nil, false
			}
		case *ssa.DebugRef:
		default:
			return nil, false
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return nil, false
	}
	return stored, stored != nil
}

// capturedReadOnlyWithin reports whether closure, and every closure nested in it
// that captures the same cell, only reads cell.
func capturedReadOnlyWithin(closure *ssa.MakeClosure, cell ssa.Value, budget *SearchBudget) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok || !budget.Spend() {
		return false
	}
	for pair := range ClosureBindingPairsWithin(function, closure, budget) {
		if pair.Binding != cell {
			continue
		}
		capture := pair.Free
		if capture.Referrers() == nil {
			continue
		}
		for use := range ReferrersWithin(capture, budget) {
			switch use := use.(type) {
			case *ssa.UnOp:
				if use.Op != token.MUL {
					return false
				}
			case *ssa.MakeClosure:
				if !capturedReadOnlyWithin(use, capture, budget) {
					return false
				}
			case *ssa.DebugRef:
			default:
				return false
			}
		}
	}
	return !budget.Exhausted() && !budget.PoolExhausted()
}
