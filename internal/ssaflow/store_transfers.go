package ssaflow

import (
	"go/types"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// Transfer evidence recognizes calls that hand a lifecycle obligation to a
// returned owner, receiver, deferred cleanup, or escaping container. A call is
// considered consuming only when its value flow and lifecycle use are visible.

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

// CallTransfersArgumentToReturnedOwner reports whether a source-visible
// callee hands the argument back inside every value it returns, and the
// caller then lets that result escape. Both halves are required: a callee
// that returns the owner on only some paths may drop it on the others, and a
// result the caller keeps local is still the caller's to release. Cilium's
// statedb wraps a response body in a returned iterator this way:
// https://github.com/cilium/statedb/blob/3546c463bfbb8afa5263b692be472bfb958bedcf/http_client.go#L78-L89
func CallTransfersArgumentToReturnedOwner(instruction ssa.Instruction, value ssa.Value) bool {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	callee := staticCalleeBody(call.Common())
	if callee == nil || !valueTransferred(call, map[ssa.Value]bool{}) {
		return false
	}
	for index, argument := range call.Common().Args {
		if index >= len(callee.Params) || !ValueDerivesFrom(argument, value, map[ssa.Value]bool{}) && !ValueContainsValue(argument, value) {
			continue
		}
		parameter := callee.Params[index]
		owned := false
		unowned := UnownedReturnFromEntryAllow(callee, func(ssa.Instruction) bool { return false }, func(returned *ssa.Return) bool {
			if ReturnedValueOwnsValue(returned, parameter) {
				owned = true
				return true
			}
			return false
		})
		if owned && !unowned {
			return true
		}
	}
	return false
}

// staticCalleeBody returns the callee whose body can be analyzed. Generic
// instantiations may carry no blocks of their own; the origin has the same
// parameter positions and the source body.
func staticCalleeBody(common *ssa.CallCommon) *ssa.Function {
	callee := common.StaticCallee()
	if callee == nil {
		return nil
	}
	if len(callee.Blocks) == 0 && callee.Origin() != nil {
		callee = callee.Origin()
	}
	if len(callee.Blocks) == 0 {
		return nil
	}
	return callee
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
				if storesParameterInReceiverField(candidate, callee.Params[0], parameter) {
					return true
				}
			}
		}
	}
	return false
}

// storesParameterInReceiverField reports whether candidate stores parameter,
// or an aggregate holding it, into a field reached from receiver. append packs
// its variadic arguments into an array before the call, so a stored slice
// contains the parameter without deriving from it operand by operand. Fabric
// appends profile closers this way:
// https://github.com/hyperledger-labs/fabric-smart-client/blob/cb202fc2768b3e72b0197bbaf401b9c2287098e8/node/start/profile/profile.go#L150-L152
func storesParameterInReceiverField(candidate ssa.Instruction, receiver, parameter ssa.Value) bool {
	store, ok := candidate.(*ssa.Store)
	if !ok {
		return false
	}
	field, ok := store.Addr.(*ssa.FieldAddr)
	if !ok || !ValueDerivesFrom(field.X, receiver, map[ssa.Value]bool{}) {
		return false
	}
	return ValueDerivesFrom(store.Val, parameter, map[ssa.Value]bool{}) || ValueContainsValue(store.Val, parameter)
}

// ValueEscapes reports whether value is transferred beyond its current
// function through a return, store, send, or escaping closure.
func ValueEscapes(value ssa.Value) bool {
	return valueTransferred(value, map[ssa.Value]bool{})
}

// CallTransfersArgumentToLifecycleOwner recognizes a consumed value only when
// the call returns an escaping object with an explicit cleanup lifecycle.
// Receiver method names are not ownership evidence. Source-visible receiver
// stores are proved separately by CallTransfersArgumentToReceiver, while
// imported receiver stores require an exported lifecycle fact.
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
	return callReturnsLifecycleOwner(call, instruction)
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
