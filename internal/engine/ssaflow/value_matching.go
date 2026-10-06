package ssaflow

import (
	"go/token"

	"github.com/kojah/gohawk/internal/engine/syntax"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"golang.org/x/tools/go/ssa"
)

// CapturedBindingValue selects a possible initial value for a capture. It
// provides no stable-content or exact asynchronous binding guarantee.
func CapturedBindingValue(binding ssa.Value) ssa.Value { //nolint:ireturn // Stored captures may contain any SSA value implementation.
	return CapturedBindingValueWithin(binding, nil)
}

// CapturedBindingValueWithin is the same possible-value selection charged to
// budget. Exhaustion returns nil; callers retain budget availability separately.
func CapturedBindingValueWithin(
	binding ssa.Value, budget *proofs.SearchBudget,
) ssa.Value { //nolint:ireturn // Stored captures may contain any SSA value implementation.
	if syntax.PointerStruct(binding.Type()) != nil {
		// A captured struct local is represented by its address. Its stores
		// initialize or mutate the value; they do not replace its identity.
		return binding
	}
	if binding.Referrers() == nil {
		return binding
	}
	for _, reference := range *binding.Referrers() {
		if !budget.Spend() {
			return nil
		}
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == binding {
			return store.Val
		}
	}
	return binding
}

// StructurallySame is the value-graph half of MayAlias: identity through
// conversions, any phi edge, and every store into a local cell, without
// regard to order. Derivation still asks this question, because its
// polarity ends the walk at anything unknown.
func StructurallySame(value, target ssa.Value) bool {
	return StructurallySameWithin(value, target, nil)
}

// StructurallySameWithin charges reaching values, address selections and store
// referrers to budget. Cutoff supplies no possible identity evidence.
func StructurallySameWithin(value, target ssa.Value, budget *proofs.SearchBudget) bool {
	// SSA removes ordinary assignments, but captured locals, embedded fields,
	// and interface conversions still need explicit identity recovery.
	forms := TransparentChangeInterface | TransparentChangeType | TransparentConvert | TransparentMakeInterface
	matched := sameStructuralValue(NewReachingWalk(forms).Within(budget), value, target) ||
		sameStructuralValue(NewReachingWalk(forms).Within(budget), target, value)
	return matched && !budget.Exhausted() && !budget.PoolExhausted()
}

// DefinitelyNil reports whether every represented SSA value is nil.
// Interface boxing remains opaque: an interface holding a typed nil is nonnil.
func DefinitelyNil(value ssa.Value) bool {
	return DefinitelyNilWithin(value, nil)
}

// DefinitelyNilWithin shares the allowance through every represented value.
// False at cutoff is unavailable, not nonnil; interface boxing stays opaque.
func DefinitelyNilWithin(value ssa.Value, budget *proofs.SearchBudget) bool {
	forms := TransparentChangeInterface | TransparentChangeType | TransparentConvert
	return NewReachingWalk(forms).Within(budget).Every(value, func(_ ReachingWalk, value ssa.Value) bool {
		literal, ok := value.(*ssa.Const)
		return ok && literal.IsNil()
	})
}

func sameStructuralValue(walk ReachingWalk, value, target ssa.Value) bool {
	if value == nil || target == nil {
		return false
	}
	// Two directional channel conversions can be siblings of the same value:
	// one producer receives chan<- T while its join helper receives <-chan T.
	// Normalize the target as the shared walk peels the value's wrappers.
	// https://github.com/Consensys/ask-o11y-plugin/blob/b74147d834cfd415caa96f087972a546238168c0/pkg/agent/loop_test.go#L111-L141
	forms := TransparentChangeInterface | TransparentChangeType | TransparentConvert | TransparentMakeInterface
	if inner, wrapped := UnwrapTransparentValue(target, forms); wrapped {
		if !walk.budget.Spend() {
			return false
		}
		target = inner
	}
	return walk.AnyIncludingOrigin(value, func(value ssa.Value) bool { return value == target }, func(walk ReachingWalk, value ssa.Value) bool {
		return structuralLeafMatches(walk, value, target)
	})
}

func structuralLeafMatches(walk ReachingWalk, value, target ssa.Value) bool {
	switch typed := value.(type) {
	case *ssa.FieldAddr:
		other, ok := target.(*ssa.FieldAddr)
		return ok && typed.Field == other.Field && sameStructuralValue(walk, typed.X, other.X)
	case *ssa.IndexAddr:
		other, ok := target.(*ssa.IndexAddr)
		return ok && sameStructuralValue(walk, typed.X, other.X) && StructurallySameWithin(typed.Index, other.Index, walk.budget)
	case *ssa.UnOp:
		if typed.Op != token.MUL {
			return false
		}
		if other, ok := target.(*ssa.UnOp); ok && other.Op == token.MUL && sameStructuralValue(walk, typed.X, other.X) {
			return true
		}
		return storedValueMatches(walk, typed.X, target)
	}
	return false
}

// storedValueMatches reports whether a value stored at exactly this address
// may be the target. A store into a field or element beneath the address is
// containment, not identity: a load of the whole aggregate carries that value
// without being it, and MayContainValue answers that question. Equating the
// two here would make an aggregate copied by value look like a possible alias
// of the resource it holds, and a proof that must keep aliases and containers
// apart would then decline the container as ambiguous.
func storedValueMatches(walk ReachingWalk, address, target ssa.Value) bool {
	if address == nil || address.Referrers() == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		if !walk.budget.Spend() {
			return false
		}
		if store, ok := reference.(*ssa.Store); ok && store.Addr == address && sameStructuralValue(walk, store.Val, target) {
			return true
		}
	}
	return false
}
