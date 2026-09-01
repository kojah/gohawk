// Package errorownership implements the errorownership gohawk analyzer.
package errorownership

import (
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
		Name: "errorownership", Doc: "checks that one function does not both log and return an error",
		Requires: []*analysis.Analyzer{buildssa.Analyzer}, Run: runErrorOwnership,
	}
}

func runErrorOwnership(pass *analysis.Pass) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
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
	for _, symbol := range []analysisutil.Symbol{
		analysisutil.PackageFunction("log", "Print"),
		analysisutil.PackageFunction("log", "Printf"),
		analysisutil.PackageFunction("log", "Println"),
		analysisutil.PackageFunction("log/slog", "Error"),
		analysisutil.PackageFunction("log/slog", "ErrorContext"),
	} {
		if ssautil.CallMatchesSymbol(common, symbol) {
			return true
		}
	}
	return false
}

func loggedErrorIsReturned(call *ssa.Call) bool {
	var logged []ssa.Value
	for _, argument := range call.Common().Args {
		if len(ssautil.ValueSources(argument)) > 0 {
			logged = append(logged, argument)
		}
	}
	for _, returned := range ssautil.ReachableReturns(call) {
		for _, result := range returned.Results {
			if !analysisutil.IsErrorType(result.Type()) {
				continue
			}
			for _, argument := range logged {
				if ssautil.ValuesShareErrorSource(argument, result) {
					return true
				}
			}
		}
	}
	return false
}
