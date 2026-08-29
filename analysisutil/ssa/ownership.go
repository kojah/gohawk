package ssautil

import "golang.org/x/tools/go/ssa"

// ClosureOwnsValue reports whether a started closure captures value.
func ClosureOwnsValue(instruction ssa.Instruction, value ssa.Value) bool {
	if _, ok := instruction.(*ssa.Go); !ok {
		return false
	}
	common := InstructionCall(instruction)
	if common == nil {
		return false
	}
	closure, ok := common.Value.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	for _, binding := range closure.Bindings {
		if CapturedBindingAliases(binding, value) {
			return true
		}
	}
	return false
}

func closureCallsValue(closure *ssa.MakeClosure, target ssa.Value) bool {
	return closureCallsCapturedValue(closure, func(binding ssa.Value) bool {
		return CapturedBindingAliases(binding, target)
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
					if index < len(closure.Bindings) && CapturedBindingAliases(binding, free) && owns(closure.Bindings[index]) {
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
				if AliasesValue(common.Value, free) && index < len(closure.Bindings) && owns(closure.Bindings[index]) {
					return true
				}
			}
		}
	}
	return false
}

// StoresValueInField reports whether instruction transfers value into a struct field.
func StoresValueInField(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok || !AliasesValue(store.Val, value) {
		return false
	}
	_, ok = store.Addr.(*ssa.FieldAddr)
	return ok
}

// StoresValueInGlobal reports whether instruction transfers value into
// package-owned storage.
func StoresValueInGlobal(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok || !AliasesValue(store.Val, value) {
		return false
	}
	_, ok = store.Addr.(*ssa.Global)
	return ok
}

// StoresOwnerOfValueInField reports whether instruction stores a callback or
// aggregate that transitively captures value into a struct field.
func StoresOwnerOfValueInField(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok {
		return false
	}
	if _, ok := store.Addr.(*ssa.FieldAddr); !ok {
		return false
	}
	return valueOwnsValue(store.Val, value, map[ssa.Value]bool{})
}

func valueOwnsValue(owner, value ssa.Value, seen map[ssa.Value]bool) bool {
	if owner == nil || seen[owner] {
		return false
	}
	if AliasesValue(owner, value) {
		return true
	}
	seen[owner] = true
	switch typed := owner.(type) {
	case *ssa.MakeClosure:
		for _, binding := range typed.Bindings {
			if CapturedBindingAliases(binding, value) || valueOwnsValue(CapturedBindingValue(binding), value, seen) {
				return true
			}
		}
	case *ssa.ChangeInterface:
		return valueOwnsValue(typed.X, value, seen)
	case *ssa.ChangeType:
		return valueOwnsValue(typed.X, value, seen)
	case *ssa.Convert:
		return valueOwnsValue(typed.X, value, seen)
	case *ssa.MakeInterface:
		return valueOwnsValue(typed.X, value, seen)
	}
	return false
}

// CallReturnsDeferredCleanup reports whether a call consumes value and one of
// its function results is subsequently deferred by the caller.
func CallReturnsDeferredCleanup(instruction ssa.Instruction, value ssa.Value) bool {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	usesValue := false
	for _, argument := range call.Common().Args {
		usesValue = usesValue || AliasesValue(argument, value)
	}
	if !usesValue || call.Referrers() == nil {
		return false
	}
	for _, reference := range *call.Referrers() {
		if deferred, ok := reference.(*ssa.Defer); ok && AliasesValue(deferred.Common().Value, call) {
			return true
		}
		result, ok := reference.(ssa.Value)
		if !ok || result.Referrers() == nil {
			continue
		}
		for _, use := range *result.Referrers() {
			deferred, ok := use.(*ssa.Defer)
			if ok && AliasesValue(deferred.Common().Value, result) {
				return true
			}
		}
	}
	return false
}

// StoresValueInOwnedMap reports whether instruction transfers value into a
// map that belongs to a caller, receiver, closure, or package owner.
func StoresValueInOwnedMap(instruction ssa.Instruction, value ssa.Value) bool {
	update, ok := instruction.(*ssa.MapUpdate)
	return ok && AliasesValue(update.Value, value) && ExternallyOwnedValue(update.Map)
}

// ExternallyOwnedValue reports whether value comes from storage that outlives
// the current function invocation.
func ExternallyOwnedValue(value ssa.Value) bool {
	return externallyOwnedValue(value, map[ssa.Value]bool{})
}

func externallyOwnedValue(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.Parameter, *ssa.FreeVar, *ssa.Global:
		return true
	case *ssa.Alloc:
		if typed.Referrers() != nil {
			for _, reference := range *typed.Referrers() {
				if store, ok := reference.(*ssa.Store); ok && store.Addr == typed && externallyOwnedValue(store.Val, seen) {
					return true
				}
			}
		}
	case *ssa.FieldAddr:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Field:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.IndexAddr:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Index:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.UnOp:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Lookup:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.ChangeInterface:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.ChangeType:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Convert:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.MakeInterface:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if externallyOwnedValue(edge, seen) {
				return true
			}
		}
	case *ssa.Slice:
		return externallyOwnedValue(typed.X, seen)
	}
	return false
}

// ClosureCapturesValue reports whether instruction creates a closure that owns value.
func ClosureCapturesValue(instruction ssa.Instruction, value ssa.Value) bool {
	closure, ok := instruction.(*ssa.MakeClosure)
	if !ok || !valueTransferred(closure, map[ssa.Value]bool{}) {
		return false
	}
	for _, binding := range closure.Bindings {
		if CapturedBindingAliases(binding, value) {
			return true
		}
	}
	return false
}

func valueTransferred(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] || value.Referrers() == nil {
		return false
	}
	seen[value] = true
	for _, reference := range *value.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Return:
			return true
		case *ssa.Store:
			if _, ok := typed.Addr.(*ssa.FieldAddr); ok {
				return true
			}
		case *ssa.ChangeInterface:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.ChangeType:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.Convert:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.Extract:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.MakeInterface:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.Phi:
			if valueTransferred(typed, seen) {
				return true
			}
		}
	}
	return false
}

// CallTransfersValueToField reports whether a call consumes value and stores
// its result in a struct field, transferring cleanup to the receiving owner.
func CallTransfersValueToField(instruction ssa.Instruction, value ssa.Value) bool {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	usesValue := false
	for _, argument := range call.Common().Args {
		usesValue = usesValue || AliasesValue(argument, value)
	}
	return usesValue && valueTransferred(call, map[ssa.Value]bool{})
}

// ReturnedValueOwnsValue reports whether a returned aggregate contains value
// in one of its fields. This recognizes constructors that transfer cleanup to
// a newly returned owner instead of returning the resource itself.
func ReturnedValueOwnsValue(returned *ssa.Return, value ssa.Value) bool {
	for _, result := range returned.Results {
		if AliasesValue(result, value) || aggregateStoresValue(result, value, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}

func aggregateStoresValue(aggregate, value ssa.Value, seen map[ssa.Value]bool) bool {
	if aggregate == nil || seen[aggregate] {
		return false
	}
	seen[aggregate] = true
	switch typed := aggregate.(type) {
	case *ssa.ChangeInterface:
		return aggregateStoresValue(typed.X, value, seen)
	case *ssa.ChangeType:
		return aggregateStoresValue(typed.X, value, seen)
	case *ssa.Convert:
		return aggregateStoresValue(typed.X, value, seen)
	case *ssa.MakeInterface:
		return aggregateStoresValue(typed.X, value, seen)
	}
	if aggregate.Referrers() == nil {
		return false
	}
	for _, reference := range *aggregate.Referrers() {
		switch typed := reference.(type) {
		case *ssa.FieldAddr:
			if addressStoresValue(typed, value) || aggregateStoresValue(typed, value, seen) {
				return true
			}
		case *ssa.IndexAddr:
			if addressStoresValue(typed, value) || aggregateStoresValue(typed, value, seen) {
				return true
			}
		}
	}
	return false
}

func addressStoresValue(address ssa.Value, value ssa.Value) bool {
	if address.Referrers() == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == address && AliasesValue(store.Val, value) {
			return true
		}
	}
	return false
}
