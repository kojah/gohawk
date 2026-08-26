package general

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/kojah/gohawk/internal/checkutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

const errorTextMatchDirective = "gohawk:error-text-match "

func errorOwnershipAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "errorownership",
		Doc:      "checks that errors are handled once and classified structurally",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runErrorOwnership,
	}
}

func runErrorOwnership(pass *analysis.Pass) (any, error) {
	for _, function := range checkutil.SourceSSAFunctions(pass) {
		file := checkutil.FunctionFile(pass, function)
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
					pass.Reportf(call.Pos(), "error is logged and returned by same function")
				}
				if !isTest && stringErrorClassificationSSA(call) && !errorTextMatchAllowedAt(pass, file, call.Pos()) {
					pass.Reportf(call.Pos(), "do not classify errors by matching Error text")
				}
			}
		}
	}
	return nil, nil
}

func errorTextMatchAllowedAt(pass *analysis.Pass, file *ast.File, position token.Pos) bool {
	line := pass.Fset.Position(position).Line
	for _, group := range file.Comments {
		firstLine := pass.Fset.Position(group.Pos()).Line
		lastLine := pass.Fset.Position(group.End()).Line
		if firstLine <= line && lastLine >= line-1 && strings.Contains(group.Text(), errorTextMatchDirective) {
			return true
		}
	}
	return false
}

func loggingCall(common *ssa.CallCommon) bool {
	name := checkutil.CallName(common)
	if checkutil.CallPackage(common) == "log" {
		return name == "Print" || name == "Printf" || name == "Println"
	}
	return checkutil.CallPackage(common) == "log/slog" && (name == "Error" || name == "ErrorContext")
}

func loggedErrorIsReturned(call *ssa.Call) bool {
	var logged []ssa.Value
	for _, argument := range call.Common().Args {
		if len(checkutil.ValueSources(argument)) > 0 {
			logged = append(logged, argument)
		}
	}
	if len(logged) == 0 {
		return false
	}
	for _, returned := range checkutil.ReachableReturns(call) {
		for _, result := range returned.Results {
			if !checkutil.IsErrorType(result.Type()) {
				continue
			}
			for _, argument := range logged {
				if checkutil.ValuesShareErrorSource(argument, result) {
					return true
				}
			}
		}
	}
	return false
}

func stringErrorClassificationSSA(call *ssa.Call) bool {
	if checkutil.CallPackage(call.Common()) != "strings" {
		return false
	}
	switch checkutil.CallName(call.Common()) {
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
		receiver := checkutil.CallReceiver(common)
		if checkutil.CallName(common) == "Error" && receiver != nil && checkutil.IsErrorType(receiver.Type()) {
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
