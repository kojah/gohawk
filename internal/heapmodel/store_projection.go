package heapmodel

import (
	"go/token"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

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
	return storage.projectionUsesPreserveStorage(address, origin, observation, seen, storage.addressProjectionUse)
}

func (storage *Storage) rootDoesNotEscapeBetween(root ssa.Value, origin, observation ssa.Instruction, seen map[ssa.Value]bool) bool {
	return storage.projectionUsesPreserveStorage(root, origin, observation, seen, storage.rootProjectionUse)
}

// Both projection boundaries use the same observation window and wrapper
// traversal. Keep depth-first visits and reject repeats: changing either can
// change which query spends the shared budget or accepts a circular proof.
func (storage *Storage) projectionUsesPreserveStorage(
	value ssa.Value, origin, observation ssa.Instruction, seen map[ssa.Value]bool,
	accept func(ssa.Value, ssa.Instruction) bool,
) bool {
	if value == nil || value.Referrers() == nil || seen[value] {
		return false
	}
	seen[value] = true
	for _, reference := range *value.Referrers() {
		if !storage.budget.Spend() {
			return false
		}
		if !instructionWithinObservation(reference, origin, observation) || accept(value, reference) {
			continue
		}
		if wrapper, ok := outwardProjectionWrapper(reference, value); ok &&
			storage.projectionUsesPreserveStorage(wrapper, origin, observation, seen, accept) {
			continue
		}
		return false
	}
	return true
}

func (storage *Storage) addressProjectionUse(address ssa.Value, reference ssa.Instruction) bool {
	switch typed := reference.(type) {
	case *ssa.DebugRef:
		return true
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return storage.effects.Call(reference, address).PreservesStorage()
	case *ssa.UnOp:
		return typed.Op == token.MUL && typed.X == address
	}
	return false
}

func (storage *Storage) rootProjectionUse(root ssa.Value, reference ssa.Instruction) bool {
	switch typed := reference.(type) {
	case *ssa.DebugRef, *ssa.FieldAddr, *ssa.IndexAddr:
		return true
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return storage.effects.Call(reference, root).PreservesStorage()
	case *ssa.BinOp:
		return (typed.Op == token.EQL || typed.Op == token.NEQ) && (ssaflow.DefinitelyNil(typed.X) || ssaflow.DefinitelyNil(typed.Y))
	}
	return false
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
	return ProveStrictProjectionPathWithin(value, root, nil).Proven()
}

// ProveStrictProjectionPathWithin shares path and stored-value visits with
// budget while retaining the default QueryBudget cap. A child cutoff remains
// unknown even if its parent still has allowance. This establishes a path,
// not stability or ownership; graph and alias internals retain separate costs.
func ProveStrictProjectionPathWithin(value, root ssa.Value, budget *ssaflow.SearchBudget) ssaflow.Proof {
	child := budget.Within(ssaflow.QueryBudget)
	depth, ok := strictAccessPathDepth(value, root, map[ssa.Value]bool{}, child)
	if child.Exhausted() || child.PoolExhausted() {
		return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
	}
	state, reason := ssaflow.EvidenceDisproven, ssaflow.EvidenceNotFound
	if ok && depth > 0 {
		state, reason = ssaflow.EvidenceProven, ssaflow.EvidenceSameAccessPath
	}
	return ssaflow.Proof{State: state, Reason: reason, Provenance: ssaflow.EvidenceFromLocalSSA}
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
