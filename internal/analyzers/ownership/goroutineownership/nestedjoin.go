package goroutineownership

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

func nestedClosureSignalValue(
	spawn *ssa.Go,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	common *ssa.CallCommon,
) ssa.Value { //nolint:ireturn // Goroutine ownership flows through concrete SSA values.
	nested, ok := common.Value.(*ssa.MakeClosure)
	if !ok {
		return nil
	}
	nestedFunction, _ := nested.Fn.(*ssa.Function)
	if nestedFunction == nil {
		return nil
	}
	for _, block := range nestedFunction.Blocks {
		for _, candidate := range block.Instrs {
			nestedSignal := closureSignalValue(nested, nestedFunction, candidate)
			if nestedSignal != nil {
				return ssaflow.SpawnedValueAtCall(spawn, function, closure, nestedSignal)
			}
		}
	}
	return nil
}

func closureSignalValue(
	closure *ssa.MakeClosure,
	function *ssa.Function,
	instruction ssa.Instruction,
) ssa.Value { //nolint:ireturn // Join handles retain their concrete SSA value types.
	var nestedValue ssa.Value
	if send, ok := instruction.(*ssa.Send); ok {
		nestedValue = send.Chan
	} else {
		common := ssaflow.InstructionCall(instruction)
		if common == nil {
			return nil
		}
		if ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")) && len(common.Args) == 1 {
			nestedValue = common.Args[0]
		} else {
			return nil
		}
	}
	return closureCapturedValue(closure, function, nestedValue)
}

func closureCapturedValue(closure *ssa.MakeClosure, function *ssa.Function, value ssa.Value) ssa.Value { //nolint:ireturn // SSA values preserve alias identity.
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) && ssaflow.ValueAliases(value, free, map[ssa.Value]bool{}) {
			return ssaflow.CapturedBindingValue(closure.Bindings[index])
		}
	}
	return nil
}
