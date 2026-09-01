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
	return deferredCallbackValueCallsMethod(common.Value, method, target, instruction, map[ssa.Value]bool{})
}

func deferredCallbackValueCallsMethod(
	value ssa.Value,
	method string,
	target ssa.Value,
	deferred ssa.Instruction,
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
		return deferredCallbackValueCallsMethod(inner, method, target, deferred, seen)
	}
	switch typed := value.(type) {
	case *ssa.MakeClosure:
		return closureCallsMethodOnEveryReturn(typed, method, target, deferred, seen)
	case *ssa.Call:
		common := typed.Common()
		return CallMatchesSymbol(common, syncOnceFunc) && len(common.Args) == 1 &&
			deferredCallbackValueCallsMethod(common.Args[0], method, target, deferred, seen)
	case *ssa.UnOp:
		return uniquelyStoredCallbackCallsMethod(typed.X, method, target, deferred, seen)
	case *ssa.Alloc:
		return uniquelyStoredCallbackCallsMethod(typed, method, target, deferred, seen)
	case *ssa.Phi:
		if len(typed.Edges) == 0 {
			return false
		}
		for _, edge := range typed.Edges {
			if !deferredCallbackValueCallsMethod(edge, method, target, deferred, cloneValueSet(seen)) {
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
	deferred ssa.Instruction,
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
				deferredCallbackValueCallsMethod(closure.Bindings[index], method, target, deferred, cloneValueSet(seen)) {
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
	deferred ssa.Instruction,
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
		if selected != nil || !InstructionDominates(store, deferred) {
			return false
		}
		selected = store
	}
	return selected != nil && deferredCallbackValueCallsMethod(selected.Val, method, target, deferred, seen)
}

func cloneValueSet(source map[ssa.Value]bool) map[ssa.Value]bool {
	result := make(map[ssa.Value]bool, len(source))
	maps.Copy(result, source)
	return result
}
