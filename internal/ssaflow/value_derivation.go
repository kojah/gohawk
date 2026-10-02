package ssaflow

import (
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// Derivation asks whether one value may have been computed from another:
// through the wrappers, stores, and aggregate addresses a caller's identity
// step accepts. It is a may-answer for suppressing a claim, not an identity
// proof.

// DerivesFrom is the walk behind ValueDerivesFrom, with the identity step
// chosen by the caller: the points-to graph's may-alias for the store
// family, the structural walk for a family beneath it.
func DerivesFrom(value, source ssa.Value, same func(ssa.Value, ssa.Value) bool) bool {
	return DerivesFromWithin(value, source, same, nil)
}

// DerivesFromWithin shares queued values, operands, aggregate address uses and
// store referrers with budget. The identity callback may share it too; its
// internals and operand allocation retain independent costs. Cutoff contributes
// no may-evidence and must remain unknown to callers, never prove absence.
func DerivesFromWithin(value, source ssa.Value, same func(ssa.Value, ssa.Value) bool, budget *SearchBudget) bool {
	if source == nil {
		return false
	}
	found := false
	WalkStatesWithin([]ssa.Value{value}, func(value ssa.Value) ssa.Value { return value }, func(value ssa.Value) ([]ssa.Value, bool) {
		if value == nil {
			return nil, true
		}
		// Identity comes before operand expansion, including for a phi.
		// ReachingWalk would expand that phi before asking its leaf predicate.
		matched := same(value, source)
		if budget.Exhausted() || budget.PoolExhausted() {
			return nil, false
		}
		if matched {
			found = true
			return nil, false
		}
		return derivationSourcesWithin(value, budget), true
	}, budget)
	return found && !budget.Exhausted() && !budget.PoolExhausted()
}

// derivationSources includes arbitrary computation operands and exact stores
// feeding a load. This is deliberately broader than transparent identity.
func derivationSourcesWithin(value ssa.Value, budget *SearchBudget) []ssa.Value {
	var sources []ssa.Value
	if load, ok := value.(*ssa.UnOp); ok && load.Op == token.MUL {
		for address := load.X; address != nil; address = enclosingAggregateAddressWithin(address, budget) {
			if !budget.Spend() {
				return nil
			}
			sources = appendStoredDerivationSourcesWithin(sources, address, budget)
		}
	}
	if instruction, ok := value.(ssa.Instruction); ok {
		for _, operand := range instruction.Operands(nil) {
			if !budget.Spend() {
				return nil
			}
			if operand != nil {
				sources = append(sources, *operand)
			}
		}
	}
	return sources
}

func appendStoredDerivationSourcesWithin(sources []ssa.Value, address ssa.Value, budget *SearchBudget) []ssa.Value {
	if address.Referrers() == nil {
		return sources
	}
	for _, reference := range *address.Referrers() {
		if !budget.Spend() {
			return nil
		}
		if store, ok := reference.(*ssa.Store); ok && store.Addr == address {
			sources = append(sources, store.Val)
		}
	}
	return sources
}

// derivesStructurally is ValueDerivesFrom with the structural identity step,
// for the value family, which sits beneath the points-to graph.
func derivesStructurally(value, source ssa.Value) bool {
	return DerivesFrom(value, source, StructurallySame)
}

// enclosingAggregateAddress returns the aggregate address a field or element
// address selects from, provided every use of that aggregate is a store of
// the whole aggregate, a load, or a selection that is itself only loaded
// from. It returns nil for any other address, and for an aggregate with a
// store into one of its fields or elements, or whose address reaches a call,
// a closure, or storage, because a load beneath such an aggregate may return
// something other than a component of a whole-aggregate store.
func enclosingAggregateAddressWithin(address ssa.Value, budget *SearchBudget) ssa.Value {
	var enclosing ssa.Value
	switch typed := address.(type) {
	case *ssa.FieldAddr:
		enclosing = typed.X
	case *ssa.IndexAddr:
		enclosing = typed.X
	default:
		return nil
	}
	if enclosing.Referrers() == nil {
		return nil
	}
	for _, reference := range *enclosing.Referrers() {
		if !budget.Spend() {
			return nil
		}
		if store, ok := reference.(*ssa.Store); ok {
			if store.Addr != enclosing {
				return nil
			}
			continue
		}
		if !addressUseOnlyLoadsWithin(reference, budget) {
			return nil
		}
	}
	return enclosing
}

// WholeWrittenCell reports whether the cell is only ever stored as a whole
// and otherwise read, directly or through field and element selections: the
// shape the builder gives a spilled by-value parameter or a local copy. Such
// a cell's contents are exactly what was stored into it.
func WholeWrittenCell(cell *ssa.Alloc) bool {
	return WholeWrittenCellWithin(cell, nil)
}

// WholeWrittenCellWithin shares whole-cell and selection-referrer visits with
// budget. Cutoff supplies no whole-written evidence; nil retains the default.
func WholeWrittenCellWithin(cell *ssa.Alloc, budget *SearchBudget) bool {
	return enclosingAggregateAddressWithin(&ssa.FieldAddr{X: cell}, budget) != nil && !budget.Exhausted() && !budget.PoolExhausted()
}

// addressOnlyLoaded reports whether an address, and every field or element
// selected beneath it, is only ever loaded from.
func addressOnlyLoadedWithin(address ssa.Value, budget *SearchBudget) bool {
	if !budget.Spend() {
		return false
	}
	if address.Referrers() == nil {
		return true
	}
	for _, reference := range *address.Referrers() {
		if !budget.Spend() {
			return false
		}
		if !addressUseOnlyLoadsWithin(reference, budget) {
			return false
		}
	}
	return true
}

// Selected addresses may only be loaded or selected further. Whole-root stores
// are a separate policy of enclosingAggregateAddressWithin, never accepted here.
// The caller charges this use; recursive selection visits retain their costs.
func addressUseOnlyLoadsWithin(reference ssa.Instruction, budget *SearchBudget) bool {
	switch typed := reference.(type) {
	case *ssa.FieldAddr:
		return addressOnlyLoadedWithin(typed, budget)
	case *ssa.IndexAddr:
		return addressOnlyLoadedWithin(typed, budget)
	case *ssa.UnOp:
		return typed.Op == token.MUL
	case *ssa.DebugRef:
		return true
	default:
		return false
	}
}
