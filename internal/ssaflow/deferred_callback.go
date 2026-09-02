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
		receiverMatches := calledReceiverMatches(nil, closure, function, CallReceiver(common), target)
		// Only deferred callbacks need the binding to remain exact through function
		// return. An immediately called closure observes its capture at this call.
		if allowOnceFunc {
			receiverMatches = deferredReceiverMatches(closure, function, CallReceiver(common), target, invocation)
		}
		if CallName(common) == method && receiverMatches {
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

func deferredReceiverMatches(
	closure *ssa.MakeClosure,
	function *ssa.Function,
	receiver ssa.Value,
	target ssa.Value,
	invocation ssa.Instruction,
) bool {
	for index, free := range function.FreeVars {
		if index >= len(closure.Bindings) || !ValueDerivesFrom(receiver, free, map[ssa.Value]bool{}) {
			continue
		}
		binding, ok := deferredBindingValue(closure.Bindings[index], target, invocation)
		if !ok {
			continue
		}
		if SameValue(binding, target) || ValueDerivesFrom(binding, target, map[ssa.Value]bool{}) || SameAccessPath(
			AccessPath{Value: receiver, Root: free},
			AccessPath{Value: target, Root: binding},
		) {
			return true
		}
	}
	return false
}

//nolint:ireturn // SSA bindings have several concrete forms.
func deferredBindingValue(binding, target ssa.Value, invocation ssa.Instruction) (ssa.Value, bool) {
	if valueHasDirectStore(binding) {
		// Addressable captures must use the unique-store proof below. General
		// identity traversal may otherwise match one historical value while the
		// deferred callback observes a later reassignment.
		return uniquelyStoredValueBefore(binding, invocation)
	}
	if SameValue(binding, target) || ValueIsAccessPathFrom(target, binding) {
		return binding, true
	}
	// Reassigned captures are addressable cells with multiple stores. A match
	// against any historical store is insufficient: deferred closures observe
	// the cell's value when they run, after assignments following the defer.
	stored, ok := uniquelyStoredValueBefore(binding, invocation)
	return stored, ok
}

func valueHasDirectStore(value ssa.Value) bool {
	if value == nil || value.Referrers() == nil {
		return false
	}
	for _, reference := range *value.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == value {
			return true
		}
	}
	return false
}

func uniquelyStoredCallbackCallsMethod(
	address ssa.Value,
	method string,
	target ssa.Value,
	invocation ssa.Instruction,
	allowOnceFunc bool,
	seen map[ssa.Value]bool,
) bool {
	stored, ok := uniquelyStoredValueBefore(address, invocation)
	return ok && callbackValueCallsMethod(stored, method, target, invocation, allowOnceFunc, seen)
}

//nolint:ireturn // Stored callbacks and captures may be any SSA value.
func uniquelyStoredValueBefore(address ssa.Value, invocation ssa.Instruction) (ssa.Value, bool) {
	if address == nil || address.Referrers() == nil {
		return nil, false
	}
	var selected *ssa.Store
	for _, reference := range *address.Referrers() {
		store, ok := reference.(*ssa.Store)
		if !ok || store.Addr != address {
			continue
		}
		if selected != nil || !InstructionDominates(store, invocation) {
			return nil, false
		}
		selected = store
	}
	if selected == nil {
		return nil, false
	}
	return selected.Val, true
}

func cloneValueSet(source map[ssa.Value]bool) map[ssa.Value]bool {
	result := make(map[ssa.Value]bool, len(source))
	maps.Copy(result, source)
	return result
}
