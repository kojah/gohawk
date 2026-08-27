package general

import (
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func errorOwnershipAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "errorownership",
		Doc:      "checks that errors are handled once and classified structurally",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runErrorOwnership,
	}
}

func runErrorOwnership(pass *analysis.Pass) (any, error) {
	for _, function := range analysisutil.SourceSSAFunctions(pass) {
		file := analysisutil.FunctionFile(pass, function)
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
				if loggingCall(call.Common()) && loggedErrorIsReturned(call) {
					analysisutil.Reportf(pass, call.Pos(), "error is logged and returned by same function")
				}
				if !isTest && stringErrorClassificationSSA(call) {
					analysisutil.Reportf(pass, call.Pos(), "do not classify errors by matching Error text")
				}
			}
		}
	}
	return nil, nil
}

func loggingCall(common *ssa.CallCommon) bool {
	name := analysisutil.CallName(common)
	if analysisutil.CallPackage(common) == "log" {
		return name == "Print" || name == "Printf" || name == "Println"
	}
	return analysisutil.CallPackage(common) == "log/slog" && (name == "Error" || name == "ErrorContext")
}

func loggedErrorIsReturned(call *ssa.Call) bool {
	var logged []ssa.Value
	for _, argument := range call.Common().Args {
		if len(analysisutil.ValueSources(argument)) > 0 {
			logged = append(logged, argument)
		}
	}
	if len(logged) == 0 {
		return false
	}
	for _, returned := range analysisutil.ReachableReturns(call) {
		for _, result := range returned.Results {
			if !analysisutil.IsErrorType(result.Type()) {
				continue
			}
			for _, argument := range logged {
				if analysisutil.ValuesShareErrorSource(argument, result) {
					return true
				}
			}
		}
	}
	return false
}

func stringErrorClassificationSSA(call *ssa.Call) bool {
	if analysisutil.CallPackage(call.Common()) != "strings" {
		return false
	}
	switch analysisutil.CallName(call.Common()) {
	case "Contains", "HasPrefix", "HasSuffix", "EqualFold":
	default:
		return false
	}
	for _, argument := range call.Common().Args {
		if containsErrorTextCall(argument, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}

func containsErrorTextCall(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if call, ok := value.(*ssa.Call); ok {
		common := call.Common()
		receiver := analysisutil.CallReceiver(common)
		if analysisutil.CallName(common) == "Error" && receiver != nil && analysisutil.IsErrorType(receiver.Type()) {
			return true
		}
	}
	instruction, ok := value.(ssa.Instruction)
	if !ok {
		return false
	}
	var operands []*ssa.Value
	operands = instruction.Operands(operands)
	for _, operand := range operands {
		if operand != nil && containsErrorTextCall(*operand, seen) {
			return true
		}
	}
	return false
}
