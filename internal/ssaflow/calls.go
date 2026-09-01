package ssaflow

import (
	"errors"

	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// SourceSSAFunctions returns non-generated source functions from buildssa results.
func SourceSSAFunctions(pass *analysis.Pass) ([]*ssa.Function, error) {
	result, ok := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if !ok {
		return nil, errors.New("buildssa prerequisite returned unexpected result")
	}
	functions := make([]*ssa.Function, 0, len(result.SrcFuncs))
	for _, function := range result.SrcFuncs {
		file := FunctionFile(pass, function)
		if function.Syntax() == nil || file == nil || !syntax.AnalyzeFile(pass, file) {
			continue
		}
		functions = append(functions, function)
	}
	return functions, nil
}

// InstructionCall returns call metadata carried by call-like SSA instructions.
func InstructionCall(instruction ssa.Instruction) *ssa.CallCommon {
	switch typed := instruction.(type) {
	case *ssa.Call:
		return typed.Common()
	case *ssa.Defer:
		return typed.Common()
	case *ssa.Go:
		return typed.Common()
	default:
		return nil
	}
}

// CallName returns a statically known method, function, or builtin name.
// Use it only for structural contracts whose receiver or package is established
// separately. Match well-known declarations with CallMatchesSymbol instead.
func CallName(common *ssa.CallCommon) string {
	if common == nil {
		return ""
	}
	if common.Method != nil {
		return common.Method.Name()
	}
	if callee := common.StaticCallee(); callee != nil {
		return callee.Name()
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		return builtin.Name()
	}
	return ""
}

// CallPackage returns a statically known package path for reporting,
// configuration, or package-wide API families. Match an exact well-known
// declaration with CallMatchesSymbol instead.
func CallPackage(common *ssa.CallCommon) string {
	if common == nil {
		return ""
	}
	if common.Method != nil && common.Method.Pkg() != nil {
		return common.Method.Pkg().Path()
	}
	callee := common.StaticCallee()
	if callee == nil {
		return ""
	}
	if object := callee.Object(); object != nil && object.Pkg() != nil {
		return object.Pkg().Path()
	}
	if callee.Pkg == nil || callee.Pkg.Pkg == nil {
		return ""
	}
	return callee.Pkg.Pkg.Path()
}

// InstructionTerminatesControlFlow reports calls whose documented behavior
// prevents execution from continuing in the current goroutine.
func InstructionTerminatesControlFlow(instruction ssa.Instruction) bool {
	common := InstructionCall(instruction)
	return HasLibraryContract(common, ContractRuntimeGoexit) || HasLibraryContract(common, ContractTestingTermination)
}

// CallInvokesArgumentOnEveryReturn reports whether a statically known helper
// invokes target on every normal path through the helper.
func CallInvokesArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value) bool {
	return callInvokesArgumentOnEveryReturn(instruction, target, map[*ssa.Function]bool{})
}

func callInvokesArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value, seen map[*ssa.Function]bool) bool {
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil || seen[common.StaticCallee()] {
		return false
	}
	seen[common.StaticCallee()] = true
	defer delete(seen, common.StaticCallee())
	return callOwnsArgumentOnEveryReturn(instruction, target, func(candidate ssa.Instruction, parameter ssa.Value) bool {
		common := InstructionCall(candidate)
		return common != nil && SameValue(common.Value, parameter) || callInvokesArgumentOnEveryReturn(candidate, parameter, seen)
	})
}

// CallCallsMethodOnArgumentOnEveryReturn reports whether a statically known
// helper calls method on target on every normal path through the helper.
func CallCallsMethodOnArgumentOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value) bool {
	return callCallsMethodOnArgumentOnEveryReturn(instruction, method, target, map[*ssa.Function]bool{})
}

func callCallsMethodOnArgumentOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value, seen map[*ssa.Function]bool) bool {
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil || seen[common.StaticCallee()] {
		return false
	}
	seen[common.StaticCallee()] = true
	defer delete(seen, common.StaticCallee())
	return callOwnsArgumentOnEveryReturn(instruction, target, func(candidate ssa.Instruction, parameter ssa.Value) bool {
		common := InstructionCall(candidate)
		return common != nil && CallName(common) == method && ValueDerivesFrom(CallReceiver(common), parameter, map[ssa.Value]bool{}) ||
			callCallsMethodOnArgumentOnEveryReturn(candidate, method, parameter, seen)
	})
}

func callOwnsArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value, owns func(ssa.Instruction, ssa.Value) bool) bool {
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil {
		return false
	}
	callee := common.StaticCallee()
	if len(callee.Blocks) == 0 {
		return false
	}
	for index, argument := range common.Args {
		if index >= len(callee.Params) || !SameValue(argument, target) && !ValueContainsValue(argument, target) {
			continue
		}
		parameter := callee.Params[index]
		if !UnownedReturnFromEntryAssumingNonNil(callee, parameter, func(candidate ssa.Instruction) bool {
			return owns(candidate, parameter)
		}) {
			return true
		}
	}
	return false
}

// CallReceiver returns receiver value for method calls and invocations.
func CallReceiver(common *ssa.CallCommon) ssa.Value { //nolint:ireturn // Call receivers have several concrete SSA value forms.
	if common == nil {
		return nil
	}
	if common.IsInvoke() {
		return common.Value
	}
	if len(common.Args) == 0 || common.Signature() == nil || common.Signature().Recv() == nil {
		return nil
	}
	return common.Args[0]
}

// CapturedBindingValue recovers value stored through an addressable closure binding.
