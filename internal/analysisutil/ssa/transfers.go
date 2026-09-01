package ssautil

import (
	"go/types"
	"strings"

	"golang.org/x/tools/go/ssa"
)

func CallReturnsDeferredCleanup(instruction ssa.Instruction, value ssa.Value) bool {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	usesValue := false
	for _, argument := range call.Common().Args {
		usesValue = usesValue || SameValue(argument, value)
	}
	if !usesValue || call.Referrers() == nil {
		return false
	}
	for _, reference := range *call.Referrers() {
		if deferred, ok := reference.(*ssa.Defer); ok && SameValue(deferred.Common().Value, call) {
			return true
		}
		result, ok := reference.(ssa.Value)
		if !ok || result.Referrers() == nil {
			continue
		}
		for _, use := range *result.Referrers() {
			deferred, ok := use.(*ssa.Defer)
			if ok && SameValue(deferred.Common().Value, result) {
				return true
			}
		}
	}
	return false
}

// StoresValueInOwnedMap reports whether instruction transfers value into a
// map that belongs to a caller, receiver, closure, or package owner.

func CallTransfersArgumentToReturnedOwner(instruction ssa.Instruction, value ssa.Value) bool {
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil {
		return false
	}
	callee := common.StaticCallee()
	for index, argument := range common.Args {
		if index >= len(callee.Params) || !ValueDerivesFrom(argument, value, map[ssa.Value]bool{}) && !ValueContainsValue(argument, value) {
			continue
		}
		for _, block := range callee.Blocks {
			for _, candidate := range block.Instrs {
				if returned, ok := candidate.(*ssa.Return); ok && ReturnedValueOwnsValue(returned, callee.Params[index]) {
					return true
				}
			}
		}
	}
	return false
}

// CallTransfersArgumentToReceiver reports whether a source-visible method
// stores an argument in a receiver that outlives the call.
func CallTransfersArgumentToReceiver(instruction ssa.Instruction, value ssa.Value) bool {
	call, ok := instruction.(*ssa.Call)
	if !ok || call.Common().StaticCallee() == nil {
		return false
	}
	common, callee := call.Common(), call.Common().StaticCallee()
	receiver := CallReceiver(common)
	if receiver == nil || len(callee.Params) == 0 || !ExternallyOwnedValue(receiver) && !valueTransferred(receiver, map[ssa.Value]bool{}) {
		return false
	}
	for index, argument := range common.Args {
		if index == 0 || index >= len(callee.Params) || !ValueDerivesFrom(argument, value, map[ssa.Value]bool{}) && !ValueContainsValue(argument, value) {
			continue
		}
		parameter := callee.Params[index]
		for _, block := range callee.Blocks {
			for _, candidate := range block.Instrs {
				store, storeOK := candidate.(*ssa.Store)
				if !storeOK {
					continue
				}
				field, fieldOK := store.Addr.(*ssa.FieldAddr)
				if fieldOK && ValueDerivesFrom(field.X, callee.Params[0], map[ssa.Value]bool{}) &&
					ValueDerivesFrom(store.Val, parameter, map[ssa.Value]bool{}) {
					return true
				}
			}
		}
	}
	return false
}

// ValueEscapes reports whether value is transferred beyond its current
// function through a return, store, send, or escaping closure.
func ValueEscapes(value ssa.Value) bool {
	return valueTransferred(value, map[ssa.Value]bool{})
}

// CallTransfersArgumentToLifecycleOwner recognizes cross-package boundaries
// when the consumed value is attached to an escaping object with an explicit
// cleanup lifecycle. Source bodies are not available to buildssa for imports,
// so both the transfer and lifecycle evidence must be visible at the callsite.
// https://github.com/flowexec/flow/blob/958773d81d410dd71e21460abb77da302617f96c/main.go#L48-L51
func CallTransfersArgumentToLifecycleOwner(instruction ssa.Instruction, value ssa.Value) bool {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	common := call.Common()
	name := strings.ToLower(CallName(common))
	if !callConsumesLifecycleValue(common, name, value) {
		return false
	}
	if callReturnsLifecycleOwner(call, instruction) {
		return true
	}
	receiver := CallReceiver(common)
	return lifecycleMutator(name) && hasLifecycleMethod(receiver) && lifecycleOwnerEscapes(receiver, instruction)
}

func callConsumesLifecycleValue(common *ssa.CallCommon, name string, value ssa.Value) bool {
	for _, argument := range common.Args {
		if SameValue(argument, value) || ValueContainsValue(argument, value) {
			return true
		}
	}
	if !strings.HasPrefix(name, "with") {
		return false
	}
	for _, argument := range common.Args {
		if hasLifecycleMethod(argument) && ValueDerivesFrom(argument, value, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}

func callReturnsLifecycleOwner(call *ssa.Call, instruction ssa.Instruction) bool {
	return hasLifecycleMethod(call) && lifecycleOwnerEscapes(call, instruction)
}

func lifecycleOwnerEscapes(owner ssa.Value, instruction ssa.Instruction) bool {
	return ExternallyOwnedValue(owner) ||
		valueTransferred(owner, map[ssa.Value]bool{}) ||
		valueLifecycleUsed(owner, instruction)
}

func lifecycleMutator(name string) bool {
	return strings.HasPrefix(name, "set") ||
		strings.HasPrefix(name, "add") ||
		strings.HasPrefix(name, "register") ||
		strings.HasPrefix(name, "own") ||
		strings.HasPrefix(name, "with")
}

func hasLifecycleMethod(value ssa.Value) bool {
	if value == nil {
		return false
	}
	methods := types.NewMethodSet(value.Type())
	for method := range methods.Methods() {
		switch method.Obj().Name() {
		case "Cancel", "Close", "Finalize", "Release", "Shutdown", "Stop":
			return true
		}
	}
	return false
}

func valueLifecycleUsed(value ssa.Value, after ssa.Instruction) bool {
	if value == nil || value.Parent() == nil {
		return false
	}
	for _, block := range value.Parent().Blocks {
		for _, instruction := range block.Instrs {
			if !InstructionMayFollow(after, instruction) {
				continue
			}
			common := InstructionCall(instruction)
			if common == nil || !ValueDerivesFrom(CallReceiver(common), value, map[ssa.Value]bool{}) {
				continue
			}
			switch CallName(common) {
			case "Cancel", "Close", "Finalize", "Release", "Shutdown", "Stop":
				return true
			}
		}
	}
	return false
}

// ReturnedValueOwnsValue reports whether a returned aggregate contains value
// in one of its fields. This recognizes constructors that transfer cleanup to
// a newly returned owner instead of returning the resource itself.
