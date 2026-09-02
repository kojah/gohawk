package lifecyclefacts

import (
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Retention evidence records that a callee may keep a parameter beyond the
// call: it stores the parameter itself outside its own locals, captures it in
// a literal, sends or returns it, appends it, or hands it to a callee that is
// opaque or itself retains. Values merely derived from the parameter, such as
// a loaded field, do not count, so a getter is not a retainer. The mask is
// over-approximate on the retaining side: consumers use it only to widen
// "unknown", never to prove ownership, so an extra Retained bit can suppress
// a diagnostic but cannot create one.

func retainedAnywhere(pass *analysis.Pass, function *ssa.Function, parameter ssa.Value) bool {
	derives := func(value ssa.Value) bool {
		return ssaflow.SameValue(value, parameter)
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if instructionRetains(pass, function, instruction, derives) {
				return true
			}
		}
	}
	return false
}

func instructionRetains(pass *analysis.Pass, function *ssa.Function, instruction ssa.Instruction, derives func(ssa.Value) bool) bool {
	switch typed := instruction.(type) {
	case *ssa.Store:
		local, ok := typed.Addr.(*ssa.Alloc)
		return derives(typed.Val) && (!ok || local.Parent() != function)
	case *ssa.MakeClosure:
		for _, binding := range typed.Bindings {
			if derives(binding) || derives(ssaflow.CapturedBindingValue(binding)) {
				return true
			}
		}
	case *ssa.Send:
		return derives(typed.X)
	case *ssa.MapUpdate:
		return derives(typed.Value)
	case *ssa.Return:
		return slices.ContainsFunc(typed.Results, derives)
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return callRetains(pass, ssaflow.InstructionCall(instruction), instruction, derives)
	}
	return false
}

// callRetains treats an opaque callee as retaining every argument, and a
// summarized callee as retaining the arguments its own summary marks.
func callRetains(pass *analysis.Pass, common *ssa.CallCommon, instruction ssa.Instruction, derives func(ssa.Value) bool) bool {
	if common == nil {
		return false
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		return builtin.Name() == "append" && anyArgument(common, derives)
	}
	callee := common.StaticCallee()
	if callee == nil || len(callee.Blocks) == 0 {
		if imported, ok := importFact(pass, instruction); ok {
			return argumentInMask(common, imported.Retained, derives)
		}
		return anyArgument(common, derives)
	}
	if imported, ok := importFact(pass, instruction); ok {
		return argumentInMask(common, imported.Retained, derives)
	}
	return anyArgument(common, derives)
}

func anyArgument(common *ssa.CallCommon, derives func(ssa.Value) bool) bool {
	return slices.ContainsFunc(common.Args, derives)
}

func argumentInMask(common *ssa.CallCommon, mask ParameterMask, derives func(ssa.Value) bool) bool {
	for index, argument := range common.Args {
		if mask.contains(index) && derives(argument) {
			return true
		}
	}
	return false
}
