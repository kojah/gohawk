package ssainfer

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func closureCallsValue(closure *ssa.MakeClosure, target ssa.Value) bool {
	return closureCallsCapturedValue(closure, func(binding ssa.Value) bool {
		return CapturedBindingMatches(binding, target)
	})
}

func closureCallsCapturedValue(closure *ssa.MakeClosure, owns func(ssa.Value) bool) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			if nested, ok := candidate.(*ssa.MakeClosure); ok && closureCallsCapturedValue(nested, func(binding ssa.Value) bool {
				for index, free := range function.FreeVars {
					if index < len(closure.Bindings) && CapturedBindingMatches(binding, free) && owns(closure.Bindings[index]) {
						return true
					}
				}
				return false
			}) {
				return true
			}
			common := ssaflow.InstructionCall(candidate)
			if common == nil {
				continue
			}
			for index, free := range function.FreeVars {
				if ValueDerivesFrom(common.Value, free, map[ssa.Value]bool{}) && index < len(closure.Bindings) && owns(closure.Bindings[index]) {
					return true
				}
			}
		}
	}
	return false
}

// StoresValueInField reports whether instruction transfers value into a struct field.

// MayContainValue reports whether owner may be an aggregate or closure that
// transitively contains value. Possible containment only: it can hide a
// diagnostic behind an opaque owner, never prove that the owner settles it.
func MayContainValue(owner, value ssa.Value) bool {
	if valueOwnsValue(owner, value, map[ssa.Value]bool{}) || newOwnershipSearch(nil).aggregateStoresValue(owner, value) {
		return true
	}
	// The graph follows containment through copies, merges, and captured
	// cells the value walk does not; the walk keeps the visible-constructor
	// cases the graph, being intraprocedural, cannot see.
	return heapmodel.Contains(owner, value)
}

// MayContainValueAt is MayContainValue asked at one instruction: whether the
// owner may hold the value when the instruction runs. A call's argument is
// judged before the call, so a callee summarized as storing the value into
// the argument does not make the argument contain it already.
func MayContainValueAt(owner, value ssa.Value, at ssa.Instruction) bool {
	if valueOwnsValue(owner, value, map[ssa.Value]bool{}) || newOwnershipSearch(nil).aggregateStoresValue(owner, value) {
		return true
	}
	contained, known := heapmodel.ContainsAt(owner, value, at)
	return known && contained
}

func valueOwnsValue(owner, value ssa.Value, seen map[ssa.Value]bool) bool {
	if owner == nil || seen[owner] {
		return false
	}
	if MayAlias(owner, value) {
		return true
	}
	seen[owner] = true
	if inner, ok := ssaflow.UnwrapTransparentValue(
		owner, ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		return valueOwnsValue(inner, value, seen)
	}
	if typed, ok := owner.(*ssa.MakeClosure); ok {
		for _, binding := range typed.Bindings {
			if CapturedBindingMatches(binding, value) || valueOwnsValue(ssaflow.CapturedBindingValue(binding), value, seen) {
				return true
			}
		}
	}
	return false
}

// CallReturnsDeferredCleanup reports whether a call consumes value and one of
// its function results is subsequently deferred by the caller.
