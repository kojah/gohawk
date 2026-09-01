// Package errorclassification implements the errorclassification gohawk analyzer.
package errorclassification

import (
	"strings"

	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"

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
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	callsites := ssautil.StaticCalls(functions)
	for _, function := range functions {
		file := ssautil.FunctionFile(pass, function)
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
		if exclusivelyErrorText(argument, map[ssa.Value]bool{}) && !externalProcessErrorText(argument, callsites, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}

func exclusivelyErrorText(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if call, ok := value.(*ssa.Call); ok {
		common := call.Common()
		receiver := ssautil.CallReceiver(common)
		if ssautil.CallName(common) == "Error" && receiver != nil && analysisutil.IsErrorType(receiver.Type()) {
			return true
		}
		// Only known text-preserving transforms carry error text. An arbitrary
		// string-producing call, or a phi that also contains such a value, leaves
		// insufficient evidence that the comparison classifies a Go error.
		if ssautil.CallPackage(common) != "strings" {
			return false
		}
		for _, argument := range common.Args {
			if analysisutil.IsStringType(argument.Type()) && exclusivelyErrorText(argument, seen) {
				return true
			}
		}
		return false
	}
	if phi, ok := value.(*ssa.Phi); ok {
		if len(phi.Edges) == 0 {
			return false
		}
		for _, edge := range phi.Edges {
			if !exclusivelyErrorText(edge, cloneSSASeen(seen)) {
				return false
			}
		}
		return true
	}
	instruction, ok := value.(ssa.Instruction)
	if !ok {
		return false
	}
	var operands []*ssa.Value
	operands = instruction.Operands(operands)
	for _, operand := range operands {
		if operand != nil && exclusivelyErrorText(*operand, seen) {
			return true
		}
	}
	return false
}

func externalProcessErrorText(value ssa.Value, callsites map[*ssa.Function][]*ssa.Call, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if call, ok := value.(*ssa.Call); ok {
		common := call.Common()
		if ssautil.CallName(common) == "Error" {
			// Matching stderr is sometimes the only contract exposed by an external
			// program; it is not evidence that code is classifying a native Go error.
			// Require every private-helper caller to carry command provenance before
			// accepting this boundary. Network Doctor wraps iproute2 stderr this way:
			// https://github.com/heymaikol/network-doctor/blob/336bff5c1fff3f4ed7e703e218b093a9be6dabfe/internal/simulation/netns_linux.go#L1197-L1225
			return externalProcessError(ssautil.CallReceiver(common), callsites, map[ssa.Value]bool{})
		}
	}
	if phi, ok := value.(*ssa.Phi); ok {
		for _, edge := range phi.Edges {
			if externalProcessErrorText(edge, callsites, seen) {
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
	for _, operand := range instruction.Operands(operands) {
		if operand != nil && externalProcessErrorText(*operand, callsites, seen) {
			return true
		}
	}
	return false
}

func externalProcessError(value ssa.Value, callsites map[*ssa.Function][]*ssa.Call, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.Parameter:
		return allParameterCallersPassExternalProcessError(typed, callsites, seen)
	case *ssa.Extract:
		call, ok := typed.Tuple.(*ssa.Call)
		return ok && functionExecutesExternalCommand(call.Common().StaticCallee())
	case *ssa.Call:
		return externalCommandCall(typed.Common()) || functionExecutesExternalCommand(typed.Common().StaticCallee())
	case *ssa.ChangeInterface:
		return externalProcessError(typed.X, callsites, seen)
	case *ssa.ChangeType:
		return externalProcessError(typed.X, callsites, seen)
	case *ssa.Convert:
		return externalProcessError(typed.X, callsites, seen)
	case *ssa.MakeInterface:
		return externalProcessError(typed.X, callsites, seen)
	case *ssa.Phi:
		if len(typed.Edges) == 0 {
			return false
		}
		for _, edge := range typed.Edges {
			if !externalProcessError(edge, callsites, cloneSSASeen(seen)) {
				return false
			}
		}
		return true
	}
	return false
}

func allParameterCallersPassExternalProcessError(parameter *ssa.Parameter, callsites map[*ssa.Function][]*ssa.Call, seen map[ssa.Value]bool) bool {
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
	for _, call := range calls {
		if index >= len(call.Common().Args) {
			return false
		}
		if !externalProcessError(call.Common().Args[index], callsites, cloneSSASeen(seen)) {
			return false
		}
	}
	return true
}

func cloneSSASeen(source map[ssa.Value]bool) map[ssa.Value]bool {
	result := make(map[ssa.Value]bool, len(source))
	for value := range source {
		result[value] = true
	}
	return result
}

func functionExecutesExternalCommand(function *ssa.Function) bool {
	if function == nil || function.Blocks == nil {
		return false
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if common := ssautil.InstructionCall(instruction); externalCommandCall(common) {
				return true
			}
		}
	}
	return false
}

func externalCommandCall(common *ssa.CallCommon) bool {
	for _, name := range []string{"Run", "Start", "Wait", "Output", "CombinedOutput"} {
		if ssautil.CallMatchesSymbol(common, analysisutil.PackageMethod("os/exec", "Cmd", name)) {
			return true
		}
	}
	return false
}

func matchesAnyFunction(common *ssa.CallCommon, packagePath string, names ...string) bool {
	for _, name := range names {
		if ssautil.CallMatchesSymbol(common, analysisutil.PackageFunction(packagePath, name)) {
			return true
		}
	}
	return false
}
