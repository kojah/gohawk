// Package errorownership implements the errorownership gohawk analyzer.
package errorownership

import (
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
		Name: "errorownership", Doc: "checks that one function does not both log and return an error",
		Requires: []*analysis.Analyzer{buildssa.Analyzer}, Run: runErrorOwnership,
	}
}

func runErrorOwnership(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if ok && loggingCall(call.Common()) && loggedErrorIsReturned(call) {
					check.Reportf(pass, check.ErrorLogAndReturn, call.Pos(), "error is logged and returned by same function")
				}
			}
		}
	}
	return nil, nil
}

func loggingCall(common *ssa.CallCommon) bool {
	for _, symbol := range []syntax.Symbol{
		syntax.PackageFunction("log", "Print"),
		syntax.PackageFunction("log", "Printf"),
		syntax.PackageFunction("log", "Println"),
		syntax.PackageFunction("log/slog", "Error"),
		syntax.PackageFunction("log/slog", "ErrorContext"),
	} {
		if ssaflow.CallMatchesSymbol(common, symbol) {
			return true
		}
	}
	return false
}

func loggedErrorIsReturned(call *ssa.Call) bool {
	var logged []ssa.Value
	for _, argument := range call.Common().Args {
		if len(ssaflow.ValueSources(argument)) > 0 {
			logged = append(logged, argument)
		}
	}
	for _, returned := range ssaflow.ReachableReturns(call) {
		for _, result := range returned.Results {
			if !syntax.IsErrorType(result.Type()) {
				continue
			}
			for _, argument := range logged {
				if ssaflow.ValuesShareErrorSource(argument, result) {
					return true
				}
			}
		}
	}
	return false
}
