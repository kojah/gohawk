package ssaflow

import (
	"iter"

	"golang.org/x/tools/go/ssa"
)

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

// InstructionsWithin yields instructions in block order, charging each one
// before yielding it. Breaking stops the census without spending on later
// instructions. Callers retain budget availability: a cutoff does not prove
// that an unvisited instruction or action is absent.
// A nil budget leaves the census unbounded.
func InstructionsWithin(function *ssa.Function, budget *SearchBudget) iter.Seq[ssa.Instruction] {
	return func(yield func(ssa.Instruction) bool) {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if !budget.Spend() || !yield(instruction) {
					return
				}
			}
		}
	}
}

// InstructionsOf collects one instruction kind through the shared census.
func InstructionsOf[T ssa.Instruction](function *ssa.Function) []T {
	var result []T
	for instruction := range InstructionsWithin(function, nil) {
		if typed, ok := instruction.(T); ok {
			result = append(result, typed)
		}
	}
	return result
}

// HasReturnAndAction reports independent witnesses for a normal return and a
// matching instruction in blocks. It proves neither ordering nor coverage;
// callers select the blocks and must still ask their path-sensitive query.
// The predicate is evaluated in block/instruction order only until its first
// match; finding a return does not stop enumeration before an action is found.
func HasReturnAndAction(blocks []*ssa.BasicBlock, action func(ssa.Instruction) bool) bool {
	hasReturn, hasAction := false, false
	for _, block := range blocks {
		for _, instruction := range block.Instrs {
			if _, ok := instruction.(*ssa.Return); ok {
				hasReturn = true
			}
			hasAction = hasAction || action(instruction)
		}
	}
	return hasReturn && hasAction
}
