package ssaflow

import "golang.org/x/tools/go/ssa"

// Instruction and closure enumeration shared by every layer.

type CapturedBinding struct {
	Free    *ssa.FreeVar
	Binding ssa.Value
}

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
