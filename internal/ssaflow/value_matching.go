package ssaflow

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

func CapturedBindingValue(binding ssa.Value) ssa.Value { //nolint:ireturn // Stored captures may contain any SSA value implementation.
	if pointer, ok := binding.Type().Underlying().(*types.Pointer); ok {
		if _, structured := pointer.Elem().Underlying().(*types.Struct); structured {
			// A captured struct local is represented by its address. Its stores
			// initialize or mutate the value; they do not replace its identity.
			return binding
		}
	}
	if binding.Referrers() == nil {
		return binding
	}
	for _, reference := range *binding.Referrers() {
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
	// SSA removes ordinary assignments, but captured locals, embedded fields,
	// and interface conversions still need explicit identity recovery.
	return sameValueSeen(value, target, map[ssa.Value]bool{}) || sameValueSeen(target, value, map[ssa.Value]bool{})
}

// DefinitelyNil reports whether every represented SSA value is nil.
func DefinitelyNil(value ssa.Value) bool {
	return definitelyNil(value, map[ssa.Value]bool{})
}

func definitelyNil(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if literal, ok := value.(*ssa.Const); ok {
		return literal.IsNil()
	}
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return definitelyNil(inner, seen)
	}
	if typed, ok := value.(*ssa.Phi); ok {
		if len(typed.Edges) == 0 {
			return false
		}
		for _, edge := range typed.Edges {
			if !definitelyNil(edge, seen) {
				return false
			}
		}
		return true
	}
	return false
}

// DeferredClosureCalls reports whether deferred closure calls method on target.

func sameValueSeen(value, target ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || target == nil {
		return false
	}
	if value == target {
		return true
	}
	if matched, wrapped := sameWrappedValue(value, target, seen); wrapped {
		return matched
	}
	if seen[value] {
		return false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.FieldAddr:
		other, ok := target.(*ssa.FieldAddr)
		return ok && typed.Field == other.Field && sameValueSeen(typed.X, other.X, seen)
	case *ssa.IndexAddr:
		other, ok := target.(*ssa.IndexAddr)
		return ok && sameValueSeen(typed.X, other.X, seen) && StructurallySame(typed.Index, other.Index)
	case *ssa.UnOp:
		if typed.Op != token.MUL {
			return false
		}
		if other, ok := target.(*ssa.UnOp); ok && other.Op == token.MUL && sameValueSeen(typed.X, other.X, seen) {
			return true
		}
		return storedValueMatches(typed.X, target, seen)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if sameValueSeen(edge, target, seen) {
				return true
			}
		}
	}
	return false
}

func sameWrappedValue(value, target ssa.Value, seen map[ssa.Value]bool) (bool, bool) {
	forms := TransparentChangeInterface | TransparentChangeType | TransparentConvert | TransparentMakeInterface
	left, leftWrapped := UnwrapTransparentValue(value, forms)
	right, rightWrapped := UnwrapTransparentValue(target, forms)
	// Two directional channel conversions can be siblings of the same value:
	// one producer receives chan<- T while its join helper receives <-chan T.
	// Both wrappers must be removed before comparing their shared identity.
	// https://github.com/Consensys/ask-o11y-plugin/blob/b74147d834cfd415caa96f087972a546238168c0/pkg/agent/loop_test.go#L111-L141
	switch {
	case leftWrapped && rightWrapped:
		return sameValueSeen(left, right, seen), true
	case leftWrapped:
		return sameValueSeen(left, target, seen), true
	case rightWrapped:
		return sameValueSeen(value, right, seen), true
	default:
		return false, false
	}
}

// storedValueMatches reports whether a value stored at exactly this address
// may be the target. A store into a field or element beneath the address is
// containment, not identity: a load of the whole aggregate carries that value
// without being it, and MayContainValue answers that question. Equating the
// two here would make an aggregate copied by value look like a possible alias
// of the resource it holds, and a proof that must keep aliases and containers
// apart would then decline the container as ambiguous.
func storedValueMatches(address, target ssa.Value, seen map[ssa.Value]bool) bool {
	if address == nil || address.Referrers() == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		if store, ok := reference.(*ssa.Store); ok && store.Addr == address && sameValueSeen(store.Val, target, seen) {
			return true
		}
	}
	return false
}
