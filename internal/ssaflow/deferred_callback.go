package ssaflow

import (
	"maps"

	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

var syncOnceFunc = syntax.PackageFunction("sync", "OnceFunc")

// Deferred callback evidence proves that the function invoked at return owns
// the requested completion. It follows only unambiguous local storage and the
// documented sync.OnceFunc callback-preservation contract.

// DeferredCallbackCallsMethod reports whether an exact deferred callback is
// structurally bound to method on target. Loads require one dominating store,
// phi edges must all settle the same target, and other call results are opaque.
func DeferredCallbackCallsMethod(instruction ssa.Instruction, method string, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common := InstructionCall(instruction)
	if common == nil {
		return false
	}
	return callbackValueCallsMethod(common.Value, method, target, instruction, true, map[ssa.Value]bool{})
}

// CalledCallbackCallsMethodOnEveryReturn reports whether an exact callback
// invoked now calls method on target before every normal return. Unlike the
// deferred form, it does not accept OnceFunc because an earlier invocation may
// already have consumed that wrapper before the current obligation exists.
func CalledCallbackCallsMethodOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value) bool {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	return callbackValueCallsMethod(call.Common().Value, method, target, instruction, false, map[ssa.Value]bool{})
}

func callbackValueCallsMethod(
	value ssa.Value,
	method string,
	target ssa.Value,
	invocation ssa.Instruction,
	allowOnceFunc bool,
	seen map[ssa.Value]bool,
) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return callbackValueCallsMethod(inner, method, target, invocation, allowOnceFunc, seen)
	}
	switch typed := value.(type) {
	case *ssa.MakeClosure:
		return closureCallsMethodOnEveryReturn(typed, method, target, invocation, allowOnceFunc, seen)
	case *ssa.Call:
		common := typed.Common()
		return allowOnceFunc && CallMatchesSymbol(common, syncOnceFunc) && len(common.Args) == 1 &&
			callbackValueCallsMethod(common.Args[0], method, target, invocation, allowOnceFunc, seen)
	case *ssa.UnOp:
		return uniquelyStoredCallbackCallsMethod(typed.X, method, target, invocation, allowOnceFunc, seen)
	case *ssa.Alloc:
		return uniquelyStoredCallbackCallsMethod(typed, method, target, invocation, allowOnceFunc, seen)
	case *ssa.Phi:
		if len(typed.Edges) == 0 {
			return false
		}
		for _, edge := range typed.Edges {
			if !callbackValueCallsMethod(edge, method, target, invocation, allowOnceFunc, cloneValueSet(seen)) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func closureCallsMethodOnEveryReturn(
	closure *ssa.MakeClosure,
	method string,
	target ssa.Value,
	invocation ssa.Instruction,
	allowOnceFunc bool,
	seen map[ssa.Value]bool,
) bool {
	function, _ := closure.Fn.(*ssa.Function)
	if function == nil || len(function.Blocks) == 0 {
		return false
	}
	callsMethod := func(instruction ssa.Instruction) bool {
		common := InstructionCall(instruction)
		if CallName(common) == method && calledReceiverMatches(nil, closure, function, CallReceiver(common), target) {
			return true
		}
		if common == nil {
			return false
		}
		for index, free := range function.FreeVars {
			if index < len(closure.Bindings) && ValueDerivesFrom(common.Value, free, map[ssa.Value]bool{}) &&
				callbackValueCallsMethod(closure.Bindings[index], method, target, invocation, allowOnceFunc, cloneValueSet(seen)) {
				return true
			}
		}
		return false
	}
	hasReturn, hasCall := false, false
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if _, ok := instruction.(*ssa.Return); ok {
				hasReturn = true
			}
			if callsMethod(instruction) {
				hasCall = true
			}
		}
	}
	return hasReturn && hasCall && !UnownedReturnFromEntry(function, callsMethod)
}

func uniquelyStoredCallbackCallsMethod(
	address ssa.Value,
	method string,
	target ssa.Value,
	invocation ssa.Instruction,
	allowOnceFunc bool,
	seen map[ssa.Value]bool,
) bool {
	if address == nil || address.Referrers() == nil {
		return false
	}
	var selected *ssa.Store
	for _, reference := range *address.Referrers() {
		store, ok := reference.(*ssa.Store)
		if !ok || store.Addr != address {
			continue
		}
		if selected != nil || !InstructionDominates(store, invocation) {
			return false
		}
		selected = store
	}
	return selected != nil && callbackValueCallsMethod(selected.Val, method, target, invocation, allowOnceFunc, seen)
}

func cloneValueSet(source map[ssa.Value]bool) map[ssa.Value]bool {
	result := make(map[ssa.Value]bool, len(source))
	maps.Copy(result, source)
	return result
}
