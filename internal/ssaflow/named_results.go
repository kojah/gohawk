package ssaflow

import "golang.org/x/tools/go/ssa"

// Named results live in cells. A return statement stores its values into
// them, the deferred calls run, and the function returns what the cells then
// hold, so a deferred literal that reads a named result sees the value the
// return statement set. These helpers find that cell and that value; what a
// deferred call does with it is the caller's question.

// NamedResultCellWithin reports whether every return reads one exact named
// result from cell, sharing instruction and result visits with budget.
// Cutoff cannot identify a result; callers must check budget availability
// before interpreting a negative answer as a completed search.
func NamedResultCellWithin(function *ssa.Function, cell *ssa.Alloc, budget *SearchBudget) (int, bool) {
	if cell.Parent() != function || function.Signature.Results().Len() == 0 {
		return 0, false
	}
	index := -1
	for instruction := range InstructionsWithin(function, budget) {
		returned, ok := instruction.(*ssa.Return)
		if !ok {
			continue
		}
		position := resultReadFromWithin(returned, cell, budget)
		if position < 0 || index >= 0 && position != index {
			return 0, false
		}
		index = position
	}
	return index, index >= 0 && !budget.Exhausted() && !budget.PoolExhausted()
}

func resultReadFromWithin(returned *ssa.Return, cell *ssa.Alloc, budget *SearchBudget) int {
	for position, result := range returned.Results {
		if !budget.Spend() {
			return -1
		}
		if load, ok := result.(*ssa.UnOp); ok && load.X == cell {
			return position
		}
	}
	return -1
}

// ValueAtReturn returns the value the return statement stores into the
// named result's cell before the deferred calls run: the last store to the
// cell in the return's own block before its RunDefers. A result set earlier,
// as a bare return leaves it, is not followed.
func ValueAtReturn(returned *ssa.Return, cell *ssa.Alloc) (ssa.Value, bool) {
	var stored ssa.Value
	for _, instruction := range returned.Block().Instrs {
		switch typed := instruction.(type) {
		case *ssa.Store:
			if typed.Addr == cell {
				stored = typed.Val
			}
		case *ssa.RunDefers:
			return stored, stored != nil
		}
	}
	return nil, false
}
