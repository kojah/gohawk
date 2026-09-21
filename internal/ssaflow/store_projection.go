package ssaflow

import (
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// DefinitelySameValueAt extends definite identity through local cells with one
// dominating store reaching each load. Address escapes, ambiguous stores, and
// writes through closures prevent a proof; arbitrary loads do not match.
func DefinitelySameValueAt(value, target ssa.Value, observation ssa.Instruction) bool {
	if DefinitelySameValue(value, target) {
		return true
	}
	forms := TransparentChangeInterface | TransparentChangeType | TransparentConvert | TransparentMakeInterface
	budget := NewSearchBudget(1000)
	var matches func(ReachingWalk, ssa.Value) bool
	matches = func(walk ReachingWalk, value ssa.Value) bool {
		if !budget.Spend() {
			return false
		}
		load, ok := value.(*ssa.UnOp)
		if !ok || load.Op != token.MUL {
			return DefinitelySameValue(value, target)
		}
		cell, ok := load.X.(*ssa.Alloc)
		if !ok || observation == nil || load.Parent() != observation.Parent() {
			return false
		}
		stored, stable := cellValueAtLoad(cell, load, budget)
		return stable && walk.Every(stored, matches)
	}
	return NewReachingWalk(forms).Every(value, matches)
}

func cellValueAtLoad(cell *ssa.Alloc, load *ssa.UnOp, budget *SearchBudget) (ssa.Value, bool) {
	if cell.Referrers() == nil {
		return nil, false
	}
	var stored *ssa.Store
	for _, use := range *cell.Referrers() {
		if !budget.Spend() {
			return nil, false
		}
		switch use := use.(type) {
		case *ssa.Store:
			if use.Addr != cell {
				return nil, false
			}
			if !InstructionMayFollow(use, load) {
				continue
			}
			if stored != nil || !InstructionDominates(use, load) {
				return nil, false
			}
			stored = use
		case *ssa.UnOp:
			if use.Op != token.MUL {
				return nil, false
			}
		case *ssa.MakeClosure:
			if !callbackCaptureReadOnly(use, cell, budget) {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	if stored == nil {
		return nil, false
	}
	return stored.Val, true
}

func immutableCallbackCell(cell *ssa.Alloc, observation ssa.Instruction, budget *SearchBudget) (ssa.Value, bool) {
	if cell.Referrers() == nil || observation == nil {
		return nil, false
	}
	var stored *ssa.Store
	for _, use := range *cell.Referrers() {
		if !budget.Spend() {
			return nil, false
		}
		switch use := use.(type) {
		case *ssa.Store:
			if stored != nil || use.Addr != cell || !InstructionDominates(use, observation) {
				return nil, false
			}
			// One SSA store may execute repeatedly. A cell allocated outside
			// that loop is not immutable across its captured callbacks.
			if BlockInCycle(use.Block()) && use.Block() != cell.Block() {
				return nil, false
			}
			stored = use
		case *ssa.UnOp:
			if use.Op != token.MUL {
				return nil, false
			}
		case *ssa.MakeClosure:
			// The closure may only read this cell. Any write through a captured
			// alias is deliberately outside this initial stable-storage model.
			if !callbackCaptureReadOnly(use, cell, budget) {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	if stored == nil {
		return nil, false
	}
	return stored.Val, true
}

func callbackCaptureReadOnly(closure *ssa.MakeClosure, cell ssa.Value, budget *SearchBudget) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	for _, pair := range ClosureBindingPairs(function, closure) {
		if !budget.Spend() {
			return false
		}
		if pair.Binding != cell || pair.Free.Referrers() == nil {
			continue
		}
		for _, access := range *pair.Free.Referrers() {
			if !budget.Spend() {
				return false
			}
			load, ok := access.(*ssa.UnOp)
			if !ok || load.Op != token.MUL {
				return false
			}
		}
	}
	return true
}

// Projection stability proves that an exact field or constant-index path still
// names storage owned by its original root at one observation. Assigning or
// exposing either the root or selected address before that observation stops
// the proof; uses after the observation do not retroactively invalidate it.

// UnmodifiedNonEmptyAccessPathAt reports whether value is a strict, non-empty
// access path from root whose selected storage cannot have been replaced before
// observation. It intentionally rejects phi-selected roots and roots without a
// source instruction, because neither supplies one exact ownership interval.
func UnmodifiedNonEmptyAccessPathAt(value, root ssa.Value, observation ssa.Instruction) bool {
	if observation == nil || root == nil || root.Parent() != observation.Parent() {
		return false
	}
	origin, ok := root.(ssa.Instruction)
	if !ok || origin.Block() == nil {
		return false
	}
	if _, ambiguous := root.(*ssa.Phi); ambiguous || !strictNonEmptyAccessPath(value, root) {
		return false
	}
	address := projectedStorageAddress(value)
	if address == nil || !projectionAddressStableBetween(address, root, origin, observation) {
		return false
	}
	return rootDoesNotEscapeBetween(root, origin, observation, map[ssa.Value]bool{})
}

func projectedStorageAddress(value ssa.Value) ssa.Value { //nolint:ireturn // SSA address forms are intentionally preserved.
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return projectedStorageAddress(inner)
	}
	switch typed := value.(type) {
	case *ssa.UnOp:
		if typed.Op == token.MUL {
			return typed.X
		}
	case *ssa.FieldAddr, *ssa.IndexAddr:
		return value
	}
	return nil
}

func projectionAddressStableBetween(address, root ssa.Value, origin, observation ssa.Instruction) bool {
	for _, block := range observation.Parent().Blocks {
		for _, instruction := range block.Instrs {
			candidate, ok := instruction.(ssa.Value)
			if !ok || !addressValue(candidate) || !SameAccessPath(
				AccessPath{Value: candidate, Root: root},
				AccessPath{Value: address, Root: root},
			) {
				continue
			}
			if !addressDoesNotEscapeBetween(candidate, origin, observation, map[ssa.Value]bool{}) {
				return false
			}
		}
	}
	return true
}

func addressValue(value ssa.Value) bool {
	switch value.(type) {
	case *ssa.FieldAddr, *ssa.IndexAddr:
		return true
	default:
		return false
	}
}

func addressDoesNotEscapeBetween(address ssa.Value, origin, observation ssa.Instruction, seen map[ssa.Value]bool) bool {
	if address == nil || address.Referrers() == nil || seen[address] {
		return false
	}
	seen[address] = true
	for _, reference := range *address.Referrers() {
		if !instructionWithinObservation(reference, origin, observation) {
			continue
		}
		switch typed := reference.(type) {
		case *ssa.DebugRef:
			continue
		case *ssa.UnOp:
			if typed.Op == token.MUL && typed.X == address {
				continue
			}
		}
		if wrapper, ok := outwardProjectionWrapper(reference, address); ok &&
			addressDoesNotEscapeBetween(wrapper, origin, observation, seen) {
			continue
		}
		return false
	}
	return true
}

func rootDoesNotEscapeBetween(root ssa.Value, origin, observation ssa.Instruction, seen map[ssa.Value]bool) bool {
	if root == nil || root.Referrers() == nil || seen[root] {
		return false
	}
	seen[root] = true
	for _, reference := range *root.Referrers() {
		if !instructionWithinObservation(reference, origin, observation) {
			continue
		}
		switch typed := reference.(type) {
		case *ssa.DebugRef, *ssa.FieldAddr, *ssa.IndexAddr:
			continue
		case *ssa.BinOp:
			if (typed.Op == token.EQL || typed.Op == token.NEQ) && (DefinitelyNil(typed.X) || DefinitelyNil(typed.Y)) {
				continue
			}
		}
		if wrapper, ok := outwardProjectionWrapper(reference, root); ok && rootDoesNotEscapeBetween(wrapper, origin, observation, seen) {
			continue
		}
		return false
	}
	return true
}

func outwardProjectionWrapper(reference ssa.Instruction, inner ssa.Value) (ssa.Value, bool) { //nolint:ireturn // Preserve the concrete SSA wrapper.
	wrapper, ok := reference.(ssa.Value)
	if !ok {
		return nil, false
	}
	unwrapped, ok := UnwrapTransparentValue(
		wrapper,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	)
	return wrapper, ok && unwrapped == inner
}

func instructionWithinObservation(candidate, origin, observation ssa.Instruction) bool {
	return candidate != nil && candidate != origin && InstructionMayFollow(origin, candidate) && InstructionMayFollow(candidate, observation)
}

func strictNonEmptyAccessPath(value, root ssa.Value) bool {
	depth, ok := strictAccessPathDepth(value, root, map[ssa.Value]bool{})
	return ok && depth > 0
}

func strictAccessPathDepth(value, root ssa.Value, seen map[ssa.Value]bool) (int, bool) {
	if value == nil || root == nil || seen[value] {
		return 0, false
	}
	if value == root {
		return 0, true
	}
	seen[value] = true
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return strictAccessPathDepth(inner, root, seen)
	}
	switch typed := value.(type) {
	case *ssa.FieldAddr:
		depth, ok := strictAccessPathDepth(typed.X, root, seen)
		return depth + 1, ok
	case *ssa.IndexAddr:
		if _, ok := constantIndex(typed.Index); !ok {
			return 0, false
		}
		depth, ok := strictAccessPathDepth(typed.X, root, seen)
		return depth + 1, ok
	case *ssa.UnOp:
		if typed.Op == token.MUL {
			if depth, ok := strictAccessPathDepth(typed.X, root, seen); ok {
				return depth, true
			}
			if !storageAddressUnaliasedBeforeLoad(typed.X, typed) {
				return 0, false
			}
			stored, ok := storedValueAt(typed.X, typed)
			if !ok {
				return 0, false
			}
			return strictAccessPathDepth(stored, root, map[ssa.Value]bool{})
		}
	}
	return 0, false
}

func storageAddressUnaliasedBeforeLoad(address ssa.Value, observation *ssa.UnOp) bool {
	if address == nil || address.Referrers() == nil || observation == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		switch typed := reference.(type) {
		case *ssa.DebugRef:
			continue
		case *ssa.Store:
			if typed.Addr == address {
				continue
			}
		case *ssa.UnOp:
			if typed.Op == token.MUL && typed.X == address {
				continue
			}
		}
		if InstructionMayFollow(reference, observation) {
			return false
		}
	}
	return true
}
