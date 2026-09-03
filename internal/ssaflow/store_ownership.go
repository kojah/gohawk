package ssaflow

import "golang.org/x/tools/go/ssa"

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
			common := InstructionCall(candidate)
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

func ValueContainsValue(owner, value ssa.Value) bool {
	return valueOwnsValue(owner, value, map[ssa.Value]bool{}) || newOwnershipSearch(nil).aggregateStoresValue(owner, value)
}

func valueOwnsValue(owner, value ssa.Value, seen map[ssa.Value]bool) bool {
	if owner == nil || seen[owner] {
		return false
	}
	if SameValue(owner, value) {
		return true
	}
	seen[owner] = true
	if inner, ok := UnwrapTransparentValue(
		owner,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return valueOwnsValue(inner, value, seen)
	}
	if typed, ok := owner.(*ssa.MakeClosure); ok {
		for _, binding := range typed.Bindings {
			if CapturedBindingMatches(binding, value) || valueOwnsValue(CapturedBindingValue(binding), value, seen) {
				return true
			}
		}
	}
	return false
}

// CallReturnsDeferredCleanup reports whether a call consumes value and one of
// its function results is subsequently deferred by the caller.
