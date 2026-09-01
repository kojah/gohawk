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
			if ValueAliases(value, free, map[ssa.Value]bool{}) && index < len(closure.Bindings) {
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
		if ValueAliases(value, parameter, map[ssa.Value]bool{}) && index < len(spawn.Common().Args) {
			return spawn.Common().Args[index]
		}
	}
	return nil
}

// ValueAliases reports whether value is a wrapped or phi-derived form of target.
func ValueAliases(value, target ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || target == nil || seen[value] {
		return false
	}
	if value == target {
		return true
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.ChangeInterface:
		return ValueAliases(typed.X, target, seen)
	case *ssa.ChangeType:
		return ValueAliases(typed.X, target, seen)
	case *ssa.Convert:
		return ValueAliases(typed.X, target, seen)
	case *ssa.MakeInterface:
		return ValueAliases(typed.X, target, seen)
	case *ssa.UnOp:
		return typed.Op == token.MUL && ValueAliases(typed.X, target, seen)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if ValueAliases(edge, target, seen) {
				return true
			}
		}
	}
	return false
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
