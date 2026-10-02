package ssaflow

import (
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// SpawnedValueAtCall resolves a spawned function value back to the value
// supplied by the parent goroutine instruction.
func SpawnedValueAtCall(
	spawn *ssa.Go,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	value ssa.Value,
) ssa.Value { //nolint:ireturn // SSA values retain their concrete representations.
	return SpawnedValueAtCallWithin(spawn, function, closure, value, nil)
}

// SpawnedValueAtCallWithin bounds possible binding selection and capture
// extraction. Exhaustion returns nil and remains distinguishable on budget;
// this query never supplies exact asynchronous identity.
func SpawnedValueAtCallWithin(
	spawn *ssa.Go, function *ssa.Function, closure *ssa.MakeClosure, value ssa.Value, budget *SearchBudget,
) ssa.Value { //nolint:ireturn // SSA values retain their concrete representations.
	bindings := CallBindings(spawn.Common(), function, closure)
	// Captures are considered before arguments, preserving the candidate
	// selection order when possible identity matches more than one binding.
	for _, binding := range bindings {
		if !budget.Spend() {
			return nil
		}
		if binding.Captured && MayAliasThroughLoadsWithin(value, binding.Local, budget) {
			captured := CapturedBindingValueWithin(binding.Supplied, budget)
			// Keep the address when the first observed value is nil. The value
			// may be assigned only after an owner closure is created, as in
			// Kubernetes test-server teardown paths.
			if DefinitelyNil(captured) {
				return binding.Supplied
			}
			return captured
		}
	}
	for _, binding := range bindings {
		if !budget.Spend() {
			return nil
		}
		if !binding.Captured && MayAliasThroughLoadsWithin(value, binding.Local, budget) {
			return binding.Supplied
		}
	}
	return nil
}

// MayAliasThroughLoads reports whether value may be target seen through
// transparent wrappers, loads, or a phi merge. It is a possible identity, not
// a proof: a load is followed without asking what the cell held at that point,
// so callers use it to find a candidate binding, never to credit an action.
func MayAliasThroughLoads(value, target ssa.Value) bool {
	return MayAliasThroughLoadsWithin(value, target, nil)
}

// MayAliasThroughLoadsWithin charges reaching-value visits to budget. A cutoff
// cannot prove that value does not possibly originate at target.
func MayAliasThroughLoadsWithin(value, target ssa.Value, budget *SearchBudget) bool {
	forms := TransparentChangeInterface | TransparentChangeType | TransparentConvert | TransparentMakeInterface
	var leaf func(ReachingWalk, ssa.Value) bool
	leaf = func(walk ReachingWalk, value ssa.Value) bool {
		if value == target {
			return true
		}
		load, ok := value.(*ssa.UnOp)
		return ok && load.Op == token.MUL && walk.Any(load.X, leaf)
	}
	return target != nil && NewReachingWalk(forms).Within(budget).Any(value, leaf)
}
