package ssaflow

import (
	"golang.org/x/tools/go/ssa"
)

// Closure evidence follows captured values through bindings, stored callbacks,
// and helper calls. These helpers accept only concrete SSA relationships and
// stop at cycles or unmodeled indirection so callers can treat a match as proof.

// CapturedBinding pairs a closure's free variable with the value bound to it
// where the closure is made.
type CapturedBinding struct {
	Free    *ssa.FreeVar
	Binding ssa.Value
}

// ClosureBindingPairs pairs the free variables of function with the bindings
// of closure. Callers pass the function whose body they will search, which
// may be the origin of an instantiated closure; the origin declares the same
// free variables in the same order. A binding list shorter than the free
// variables yields only the pairs that exist, and a nil closure yields none.
func ClosureBindingPairs(function *ssa.Function, closure *ssa.MakeClosure) []CapturedBinding {
	if function == nil || closure == nil {
		return nil
	}
	pairs := make([]CapturedBinding, 0, len(function.FreeVars))
	for index, free := range function.FreeVars {
		if index >= len(closure.Bindings) {
			break
		}
		pairs = append(pairs, CapturedBinding{Free: free, Binding: closure.Bindings[index]})
	}
	return pairs
}

// InstructionsOf returns every instruction of one concrete type in function,
// in block order.
func InstructionsOf[T ssa.Instruction](function *ssa.Function) []T {
	var result []T
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if typed, ok := instruction.(T); ok {
				result = append(result, typed)
			}
		}
	}
	return result
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
			for _, captured := range ClosureBindingPairs(function, closure) {
				if CapturedBindingMatches(captured.Binding, target) && CallInvokesArgumentOnEveryReturn(candidate, captured.Free) {
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

// calledFunction returns the call, the function literal when the callee is
// one, and the callee body of a call-like instruction.
func calledFunction(instruction ssa.Instruction) (*ssa.CallCommon, *ssa.MakeClosure, *ssa.Function) {
	common := InstructionCall(instruction)
	if common == nil {
		return nil, nil, nil
	}
	closure, _ := common.Value.(*ssa.MakeClosure)
	function := common.StaticCallee()
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	return common, closure, function
}
