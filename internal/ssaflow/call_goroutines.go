package ssaflow

import (
	"go/token"
	"slices"

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
	if closure != nil {
		for index, free := range function.FreeVars {
			if MayAliasThroughLoads(value, free) && index < len(closure.Bindings) {
				captured := CapturedBindingValue(closure.Bindings[index])
				// Keep the address when the first observed value is nil. The value
				// may be assigned only after an owner closure is created, as in
				// Kubernetes test-server teardown paths.
				if DefinitelyNil(captured) {
					return closure.Bindings[index]
				}
				return captured
			}
		}
	}
	for index, parameter := range function.Params {
		if MayAliasThroughLoads(value, parameter) && index < len(spawn.Common().Args) {
			return spawn.Common().Args[index]
		}
	}
	return nil
}

// MayAliasThroughLoads reports whether value may be target seen through
// transparent wrappers, loads, or a phi merge. It is a possible identity, not
// a proof: a load is followed without asking what the cell held at that point,
// so callers use it to find a candidate binding, never to credit an action.
func MayAliasThroughLoads(value, target ssa.Value) bool {
	forms := TransparentChangeInterface | TransparentChangeType | TransparentConvert | TransparentMakeInterface
	var leaf func(ReachingWalk, ssa.Value) bool
	leaf = func(walk ReachingWalk, value ssa.Value) bool {
		if value == target {
			return true
		}
		load, ok := value.(*ssa.UnOp)
		return ok && load.Op == token.MUL && walk.Any(load.X, leaf)
	}
	return target != nil && NewReachingWalk(forms).Any(value, leaf)
}

// BlockInCycle reports whether control flow can return to start.
func BlockInCycle(start *ssa.BasicBlock) bool {
	seen := map[*ssa.BasicBlock]bool{}
	queue := slices.Clone(start.Succs)
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == start {
			return true
		}
		if seen[block] {
			continue
		}
		seen[block] = true
		queue = append(queue, block.Succs...)
	}
	return false
}
