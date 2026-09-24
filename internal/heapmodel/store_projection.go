package heapmodel

import (
	"go/token"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func callbackCaptureReadOnly(closure *ssa.MakeClosure, cell ssa.Value, budget *ssaflow.SearchBudget) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	query := ssaflow.NewCallEffects(budget)
	for _, pair := range ssaflow.ClosureBindingPairs(function, closure) {
		if !budget.Spend() {
			return false
		}
		if pair.Binding != cell {
			continue
		}
		if !query.Value(pair.Free).PreservesStorage() {
			return false
		}
	}
	return true
}

// Projection stability proves that an exact field or constant-index path still
// names storage owned by its original root at one observation. Assigning or
// exposing either the root or selected address before that observation stops
// the proof; uses after the observation do not retroactively invalidate it.

// Projection proves that value is a strict, non-empty
// access path from root whose selected storage cannot have been replaced before
// observation. It intentionally rejects phi-selected roots and roots without a
// source instruction, because neither supplies one exact ownership interval.
func (storage *Storage) Projection(value, root ssa.Value, observation ssa.Instruction) ssaflow.IdentityProof {
	if storage.unmodifiedProjection(value, root, observation) {
		return ssaflow.IdentityProof{Proof: ssaflow.Proof{
			State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceSameAccessPath, Provenance: ssaflow.EvidenceFromLocalSSA,
		}}
	}
	return ssaflow.IdentityProof{Proof: storage.unknown(ssaflow.EvidenceStorageProjectionModified, observation).Proof}
}

func (storage *Storage) unmodifiedProjection(value, root ssa.Value, observation ssa.Instruction) bool {
	if observation == nil || root == nil || root.Parent() != observation.Parent() {
		return false
	}
	origin, ok := root.(ssa.Instruction)
	if !ok || origin.Block() == nil {
		return false
	}
	if _, ambiguous := root.(*ssa.Phi); ambiguous || !StrictProjectionPath(value, root) {
		return false
	}
	address := projectedStorageAddress(value)
	if address == nil || !storage.projectionAddressStableBetween(address, root, origin, observation) {
		return false
	}
	return storage.rootDoesNotEscapeBetween(root, origin, observation, map[ssa.Value]bool{})
}

func projectedStorageAddress(value ssa.Value) ssa.Value { //nolint:ireturn // SSA address forms are intentionally preserved.
	if inner, ok := ssaflow.UnwrapTransparentValue(
		value, ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
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

func (storage *Storage) projectionAddressStableBetween(address, root ssa.Value, origin, observation ssa.Instruction) bool {
	for _, block := range observation.Parent().Blocks {
		for _, instruction := range block.Instrs {
			if !storage.budget.Spend() {
				return false
			}
			candidate, ok := instruction.(ssa.Value)
			if !ok || !addressValue(candidate) || !ssaflow.SameAccessPath(
				ssaflow.AccessPath{Value: candidate, Root: root}, ssaflow.AccessPath{Value: address, Root: root},
			) {
				continue
			}
			if !storage.addressDoesNotEscapeBetween(candidate, origin, observation, map[ssa.Value]bool{}) {
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

func (storage *Storage) addressDoesNotEscapeBetween(address ssa.Value, origin, observation ssa.Instruction, seen map[ssa.Value]bool) bool {
	if address == nil || address.Referrers() == nil || seen[address] {
		return false
	}
	seen[address] = true
	for _, reference := range *address.Referrers() {
		if !storage.budget.Spend() {
			return false
		}
		if !instructionWithinObservation(reference, origin, observation) {
			continue
		}
		switch typed := reference.(type) {
		case *ssa.DebugRef:
			continue
		case *ssa.Call, *ssa.Defer, *ssa.Go:
			if storage.effects.Call(reference, address).PreservesStorage() {
				continue
			}
		case *ssa.UnOp:
			if typed.Op == token.MUL && typed.X == address {
				continue
			}
		}
		if wrapper, ok := outwardProjectionWrapper(reference, address); ok &&
			storage.addressDoesNotEscapeBetween(wrapper, origin, observation, seen) {
			continue
		}
		return false
	}
	return true
}

func (storage *Storage) rootDoesNotEscapeBetween(root ssa.Value, origin, observation ssa.Instruction, seen map[ssa.Value]bool) bool {
	if root == nil || root.Referrers() == nil || seen[root] {
		return false
	}
	seen[root] = true
	for _, reference := range *root.Referrers() {
		if !storage.budget.Spend() {
			return false
		}
		if !instructionWithinObservation(reference, origin, observation) {
			continue
		}
		switch typed := reference.(type) {
		case *ssa.DebugRef, *ssa.FieldAddr, *ssa.IndexAddr:
			continue
		case *ssa.Call, *ssa.Defer, *ssa.Go:
			if storage.effects.Call(reference, root).PreservesStorage() {
				continue
			}
		case *ssa.BinOp:
			if (typed.Op == token.EQL || typed.Op == token.NEQ) && (ssaflow.DefinitelyNil(typed.X) || ssaflow.DefinitelyNil(typed.Y)) {
				continue
			}
		}
		if wrapper, ok := outwardProjectionWrapper(reference, root); ok && storage.rootDoesNotEscapeBetween(wrapper, origin, observation, seen) {
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
	unwrapped, ok := ssaflow.UnwrapTransparentValue(
		wrapper, ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	)
	return wrapper, ok && unwrapped == inner
}

func instructionWithinObservation(candidate, origin, observation ssa.Instruction) bool {
	return candidate != nil && candidate != origin && ssaflow.InstructionMayFollow(origin, candidate) && ssaflow.InstructionMayFollow(candidate, observation)
}

// StrictProjectionPath proves a non-empty field or constant-index path from
// root, resolving local loads where necessary. It does not establish that the
// selected storage remains unchanged at a later observation; use Projection
// for that stronger question.
func StrictProjectionPath(value, root ssa.Value) bool {
	depth, ok := strictAccessPathDepth(value, root, map[ssa.Value]bool{}, ssaflow.NewSearchBudget(ssaflow.QueryBudget))
	return ok && depth > 0
}

func strictAccessPathDepth(value, root ssa.Value, seen map[ssa.Value]bool, budget *ssaflow.SearchBudget) (int, bool) {
	if value == nil || root == nil || seen[value] || !budget.Spend() {
		return 0, false
	}
	if value == root {
		return 0, true
	}
	seen[value] = true
	if inner, ok := ssaflow.UnwrapTransparentValue(
		value, ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		return strictAccessPathDepth(inner, root, seen, budget)
	}
	switch typed := value.(type) {
	case *ssa.FieldAddr:
		depth, ok := strictAccessPathDepth(typed.X, root, seen, budget)
		return depth + 1, ok
	case *ssa.IndexAddr:
		if _, ok := ssaflow.ConstantIndex(typed.Index); !ok {
			return 0, false
		}
		depth, ok := strictAccessPathDepth(typed.X, root, seen, budget)
		return depth + 1, ok
	case *ssa.UnOp:
		if typed.Op == token.MUL {
			if depth, ok := strictAccessPathDepth(typed.X, root, seen, budget); ok {
				return depth, true
			}
			stored := NewStorage(budget).Content(typed.X, typed)
			if !stored.Proven() {
				return 0, false
			}
			return strictAccessPathDepth(stored.Value, root, seen, budget)
		}
	}
	return 0, false
}
