// Package errorclassification implements the errorclassification gohawk analyzer.
package errorclassification

import (
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "errorclassification",
		Doc:      "checks that native Go errors are classified structurally",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runErrorClassification,
	}
}

func runErrorClassification(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	callsites := ssaflow.StaticCalls(functions)
	for _, function := range functions {
		file := ssaflow.FunctionFile(pass, function)
		if file == nil {
			continue
		}
		isTest := strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go")
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok {
					continue
				}
				if !isTest && stringErrorClassificationSSA(call, callsites) {
					check.Reportf(pass, check.ErrorTextClassification, call.Pos(), "do not classify errors by matching Error text")
				}
			}
		}
	}
	return nil, nil
}

func stringErrorClassificationSSA(call *ssa.Call, callsites map[*ssa.Function][]*ssa.Call) bool {
	if !matchesAnyFunction(call.Common(), "strings", "Contains", "HasPrefix", "HasSuffix", "EqualFold") {
		return false
	}
	for _, argument := range call.Common().Args {
		if exclusivelyErrorText(ssaflow.NewReachingWalk(0), argument) && !externalProcessErrorText(ssaflow.NewReachingWalk(0), argument, callsites) {
			return true
		}
	}
	return false
}

// exclusivelyErrorText reports whether every value merged into value is the
// text of a Go error. No SSA wrapper is transparent here: a conversion of an
// error's text is no longer evidence that the comparison classifies the error.
func exclusivelyErrorText(walk ssaflow.ReachingWalk, value ssa.Value) bool {
	return walk.Every(value, exclusivelyErrorTextLeaf)
}

func exclusivelyErrorTextLeaf(walk ssaflow.ReachingWalk, value ssa.Value) bool {
	if call, ok := value.(*ssa.Call); ok {
		common := call.Common()
		receiver := ssaflow.CallReceiver(common)
		if ssaflow.CallName(common) == "Error" && receiver != nil && syntax.IsErrorType(receiver.Type()) {
			return true
		}
		// Only known text-preserving transforms carry error text. An arbitrary
		// string-producing call, or a phi that also contains such a value, leaves
		// insufficient evidence that the comparison classifies a Go error.
		if ssaflow.CallPackage(common) != "strings" {
			return false
		}
		for _, argument := range common.Args {
			if syntax.IsStringType(argument.Type()) && exclusivelyErrorText(walk, argument) {
				return true
			}
		}
		return false
	}
	instruction, ok := value.(ssa.Instruction)
	if !ok {
		return false
	}
	var operands []*ssa.Value
	operands = instruction.Operands(operands)
	for _, operand := range operands {
		if operand != nil && exclusivelyErrorText(walk, *operand) {
			return true
		}
	}
	return false
}

// externalProcessErrorText reports whether some value merged into value is
// the text of an error produced by an external command.
func externalProcessErrorText(walk ssaflow.ReachingWalk, value ssa.Value, callsites map[*ssa.Function][]*ssa.Call) bool {
	return walk.Any(value, func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		if call, ok := value.(*ssa.Call); ok {
			common := call.Common()
			if ssaflow.CallName(common) == "Error" {
				// Matching stderr is sometimes the only contract exposed by an external
				// program; it is not evidence that code is classifying a native Go error.
				// Require every private-helper caller to carry command provenance before
				// accepting this boundary. Network Doctor wraps iproute2 stderr this way:
				// https://github.com/heymaikol/network-doctor/blob/336bff5c1fff3f4ed7e703e218b093a9be6dabfe/internal/simulation/netns_linux.go#L1197-L1225
				return externalProcessError(ssaflow.NewReachingWalk(externalErrorForms), ssaflow.CallReceiver(common), callsites)
			}
		}
		instruction, ok := value.(ssa.Instruction)
		if !ok {
			return false
		}
		var operands []*ssa.Value
		for _, operand := range instruction.Operands(operands) {
			if operand != nil && externalProcessErrorText(walk, *operand, callsites) {
				return true
			}
		}
		return false
	})
}

// externalErrorForms are the wrappers an external command's error keeps its
// provenance through.
const externalErrorForms = ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface

// externalProcessError reports whether every value merged into value is an
// error produced by an external command.
func externalProcessError(walk ssaflow.ReachingWalk, value ssa.Value, callsites map[*ssa.Function][]*ssa.Call) bool {
	return walk.Every(value, func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		switch typed := value.(type) {
		case *ssa.Parameter:
			return allParameterCallersPassExternalProcessError(walk, typed, callsites)
		case *ssa.Extract:
			call, ok := typed.Tuple.(*ssa.Call)
			return ok && functionExecutesExternalCommand(call.Common().StaticCallee())
		case *ssa.Call:
			return externalCommandCall(typed.Common()) || functionExecutesExternalCommand(typed.Common().StaticCallee())
		}
		return false
	})
}

func allParameterCallersPassExternalProcessError(walk ssaflow.ReachingWalk, parameter *ssa.Parameter, callsites map[*ssa.Function][]*ssa.Call) bool {
	function := parameter.Parent()
	if function == nil || function.Object() == nil || function.Object().Exported() {
		return false
	}
	index := -1
	for candidate, current := range function.Params {
		if current == parameter {
			index = candidate
			break
		}
	}
	if index < 0 {
		return false
	}
	calls := callsites[function]
	if len(calls) == 0 {
		return false
	}
	arguments := make([]ssa.Value, 0, len(calls))
	for _, call := range calls {
		if index >= len(call.Common().Args) {
			return false
		}
		arguments = append(arguments, call.Common().Args[index])
	}
	return walk.EveryOf(arguments, func(walk ssaflow.ReachingWalk, argument ssa.Value) bool {
		return externalProcessError(walk, argument, callsites)
	})
}

func functionExecutesExternalCommand(function *ssa.Function) bool {
	if function == nil || function.Blocks == nil {
		return false
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if common := ssaflow.InstructionCall(instruction); externalCommandCall(common) {
				return true
			}
		}
	}
	return false
}

func externalCommandCall(common *ssa.CallCommon) bool {
	for _, name := range []string{"Run", "Start", "Wait", "Output", "CombinedOutput"} {
		if ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os/exec", Receiver: "Cmd", Name: name})) {
			return true
		}
	}
	return false
}

func matchesAnyFunction(common *ssa.CallCommon, packagePath string, names ...string) bool {
	for _, name := range names {
		if ssaflow.CallMatchesSymbol(common, syntax.PackageFunction(packagePath, name)) {
			return true
		}
	}
	return false
}
