package ssaflow

import "golang.org/x/tools/go/ssa"

// Named results live in cells. A return statement stores its values into
// them, the deferred calls run, and the function returns what the cells then
// hold, so a deferred literal that reads a named result sees the value the
// return statement set. These helpers find that cell and that value; what a
// deferred call does with it is the caller's question.

// NamedResultCellsProof contains cells read at the same first result position
// on every normal return. Proven means the census completed, including an
// empty Cells map; it does not prove any deferred action or stored value.
// Cutoff publishes no cells. The map belongs to this proof and is unordered.
type NamedResultCellsProof struct {
	Proof
	Cells map[*ssa.Alloc]int
}

// ProveNamedResultCellsWithin shares one instruction/result census across all
// candidate cells. Only direct reads of cells owned by function are recognized;
// wrappers and earlier stored result values remain outside this query. Each
// return's first read decides a cell's slot, preserving duplicate-read policy.
// Instruction, result and intersection visits share budget; nil is unbounded.
func ProveNamedResultCellsWithin(function *ssa.Function, budget *SearchBudget) NamedResultCellsProof {
	unknown := NamedResultCellsProof{Proof: Proof{Reason: EvidenceBudgetExhausted}}
	if !budget.Spend() {
		return unknown
	}
	if function == nil {
		return NamedResultCellsProof{Proof: Proof{Reason: EvidenceUnavailable}}
	}
	complete := NamedResultCellsProof{Proof: Proof{State: EvidenceProven, Reason: EvidenceStructuralWalk, Provenance: EvidenceFromLocalSSA}}
	if function.Signature.Results().Len() == 0 {
		return complete
	}
	var cells map[*ssa.Alloc]int
	for instruction := range InstructionsWithin(function, budget) {
		returned, ok := instruction.(*ssa.Return)
		if !ok {
			continue
		}
		current := resultCellsWithin(function, returned, budget)
		if cells == nil {
			cells = current
		} else {
			for cell, slot := range cells {
				if !budget.Spend() {
					return unknown
				}
				if position, found := current[cell]; !found || position != slot {
					delete(cells, cell)
				}
			}
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return unknown
	}
	complete.Cells = cells
	return complete
}

func resultCellsWithin(function *ssa.Function, returned *ssa.Return, budget *SearchBudget) map[*ssa.Alloc]int {
	cells := make(map[*ssa.Alloc]int)
	for position, result := range returned.Results {
		if !budget.Spend() {
			return nil
		}
		load, ok := result.(*ssa.UnOp)
		if !ok {
			continue
		}
		cell, ok := load.X.(*ssa.Alloc)
		if !ok || cell.Parent() != function {
			continue
		}
		if _, found := cells[cell]; !found {
			cells[cell] = position
		}
	}
	return cells
}

// ValueAtReturnWithin returns the exact cell's last store in the return block
// before RunDefers, charging each inspected instruction to budget. A result
// set in an earlier block is not followed. Cutoff discards the selected value;
// callers inspect budget before treating absence as a completed lookup.
// A nil budget retains the unbounded lookup.
func ValueAtReturnWithin(returned *ssa.Return, cell *ssa.Alloc, budget *SearchBudget) (ssa.Value, bool) {
	var stored ssa.Value
	for _, instruction := range returned.Block().Instrs {
		if !budget.Spend() {
			return nil, false
		}
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
