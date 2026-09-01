package ssaflow

import (
	"slices"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// Closure evidence follows captured values through bindings, stored callbacks,
// and helper calls. These helpers accept only concrete SSA relationships and
// stop at cycles or unmodeled indirection so callers can treat a match as proof.

func DeferredClosureCalls(instruction ssa.Instruction, method string, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	return ClosureCallsMethod(instruction, method, target)
}

// DeferredClosureCallsValue reports whether a deferred closure calls target.
func DeferredClosureCallsValue(instruction ssa.Instruction, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	return ClosureCallsValue(instruction, target)
}

// DeferredClosureInvokesArgumentOnEveryReturn reports whether a deferred
// closure delegates target to a helper that invokes it on every normal path.
func DeferredClosureInvokesArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common, closure, function := calledFunction(instruction)
	if function == nil {
		return false
	}
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			for index, free := range function.FreeVars {
				if index < len(closure.Bindings) && CapturedBindingMatches(closure.Bindings[index], target) &&
					CallInvokesArgumentOnEveryReturn(candidate, free) {
					return true
				}
			}
			for index, parameter := range function.Params {
				if common != nil && index < len(common.Args) && SameValue(common.Args[index], target) &&
					CallInvokesArgumentOnEveryReturn(candidate, parameter) {
					return true
				}
			}
		}
	}
	return false
}

// DeferredHelperInvokesBoundMethodOnEveryReturn reports whether a deferred
// static helper receives a callback bound to method on target and invokes that
// exact callback on every normal return. Both the defer and the helper's
// unconditional invocation are required: merely observing or conditionally
// invoking a cleanup callback does not settle the lifecycle.
func DeferredHelperInvokesBoundMethodOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil {
		return false
	}
	for _, argument := range common.Args {
		if ValueCallsMethod(argument, method, target) && CallInvokesArgumentOnEveryReturn(instruction, argument) {
			return true
		}
	}
	return false
}

// DeferredClosurePassesValueToNamedCall reports whether a deferred closure
// passes target to a call whose name contains one of fragments.
func DeferredClosurePassesValueToNamedCall(instruction ssa.Instruction, target ssa.Value, fragments ...string) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common, closure, function := calledFunction(instruction)
	if function == nil {
		return false
	}
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			called := InstructionCall(candidate)
			name := strings.ToLower(CallName(called))
			if called == nil || !slices.ContainsFunc(fragments, func(fragment string) bool {
				return strings.Contains(name, fragment)
			}) {
				continue
			}
			for _, argument := range called.Args {
				for index, free := range function.FreeVars {
					if index < len(closure.Bindings) && ValueDerivesFrom(argument, free, map[ssa.Value]bool{}) &&
						CapturedBindingMatches(closure.Bindings[index], target) {
						return true
					}
				}
				for index, parameter := range function.Params {
					if common != nil && index < len(common.Args) && ValueDerivesFrom(argument, parameter, map[ssa.Value]bool{}) &&
						SameValue(common.Args[index], target) {
						return true
					}
				}
			}
		}
	}
	return false
}

// ClosureCallsValue reports whether a call-like closure or created callback calls target.
func ClosureCallsValue(instruction ssa.Instruction, target ssa.Value) bool {
	var closure *ssa.MakeClosure
	if created, ok := instruction.(*ssa.MakeClosure); ok {
		if created.Referrers() == nil || len(*created.Referrers()) == 0 {
			return false
		}
		closure = created
	} else if common := InstructionCall(instruction); common != nil {
		closure, _ = common.Value.(*ssa.MakeClosure)
	}
	if closure == nil {
		return false
	}
	return closureCallsValue(closure, target)
}

// ValueCallsValue reports whether value is, or wraps, a callback that invokes
// target. It follows common callback wrappers and addressable locals so callers
// can recognize cleanup registered through higher-order APIs.
func ValueCallsValue(value, target ssa.Value) bool {
	return valueCallsValue(value, target, map[ssa.Value]bool{})
}

// ValueCallsMethod reports whether value is, or wraps, a callback that invokes
// method on target.
func ValueCallsMethod(value ssa.Value, method string, target ssa.Value) bool {
	return valueCallsMethod(value, method, target, map[ssa.Value]bool{})
}

func valueCallsMethod(value ssa.Value, method string, target ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if closure, ok := value.(*ssa.MakeClosure); ok && closureValueCallsMethod(closure, method, target, seen) {
		return true
	}
	if inner, ok := wrappedValue(value); ok {
		return valueCallsMethod(inner, method, target, seen)
	}
	switch typed := value.(type) {
	case *ssa.Alloc:
		return storedCallbackCallsMethod(typed, method, target, seen)
	case *ssa.Call:
		for _, argument := range typed.Common().Args {
			if valueCallsMethod(argument, method, target, seen) {
				return true
			}
		}
	case *ssa.UnOp:
		return storedCallbackCallsMethod(typed.X, method, target, seen)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if valueCallsMethod(edge, method, target, seen) {
				return true
			}
		}
	}
	return false
}

func closureValueCallsMethod(closure *ssa.MakeClosure, method string, target ssa.Value, seen map[ssa.Value]bool) bool {
	if ClosureCallsMethod(closure, method, target) {
		return true
	}
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if nested, ok := instruction.(*ssa.MakeClosure); ok && valueCallsMethod(nested, method, target, seen) {
				return true
			}
			if closureBindingCallsMethod(instruction, function, closure, method, target, seen) {
				return true
			}
		}
	}
	return false
}

func closureBindingCallsMethod(
	instruction ssa.Instruction,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	method string,
	target ssa.Value,
	seen map[ssa.Value]bool,
) bool {
	common := InstructionCall(instruction)
	if common == nil {
		return false
	}
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) &&
			ValueDerivesFrom(common.Value, free, map[ssa.Value]bool{}) &&
			valueCallsMethod(closure.Bindings[index], method, target, seen) {
			return true
		}
	}
	return false
}

func storedCallbackCallsMethod(address ssa.Value, method string, target ssa.Value, seen map[ssa.Value]bool) bool {
	if address.Referrers() == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == address && valueCallsMethod(store.Val, method, target, seen) {
			return true
		}
	}
	return false
}

func valueCallsValue(value, target ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if closure, ok := value.(*ssa.MakeClosure); ok && closureValueCallsValue(closure, target, seen) {
		return true
	}
	if inner, ok := wrappedValue(value); ok {
		return valueCallsValue(inner, target, seen)
	}
	switch typed := value.(type) {
	case *ssa.Alloc:
		return storedCallbackCallsValue(typed, target, seen)
	case *ssa.UnOp:
		return storedCallbackCallsValue(typed.X, target, seen)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if valueCallsValue(edge, target, seen) {
				return true
			}
		}
	}
	return false
}

func closureValueCallsValue(closure *ssa.MakeClosure, target ssa.Value, seen map[ssa.Value]bool) bool {
	if closureCallsValue(closure, target) {
		return true
	}
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			inner, ok := instruction.(*ssa.MakeClosure)
			if ok && valueCallsValue(inner, target, seen) {
				return true
			}
		}
	}
	return false
}

func storedCallbackCallsValue(address, target ssa.Value, seen map[ssa.Value]bool) bool {
	if address.Referrers() == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == address && valueCallsValue(store.Val, target, seen) {
			return true
		}
	}
	return false
}
