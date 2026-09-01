// Package errorownership implements the errorownership gohawk analyzer.
package errorownership

import (
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

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
				if !ok || !loggingCall(call.Common()) {
					continue
				}
				sharedSource, feasiblePath := loggedErrorReturnEvidence(call)
				if !sharedSource {
					continue
				}
				if !feasiblePath {
					traceLogReturnDecision(pass, call, "no-feasible-log-return-path", analysisTrace.OutcomeAccepted)
					continue
				}
				traceLogReturnDecision(pass, call, "feasible-log-return-path", analysisTrace.OutcomeObserved)
				check.Reportf(pass, check.ErrorLogAndReturn, call.Pos(), "error is logged and returned by same function")
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

func loggedErrorReturnEvidence(call *ssa.Call) (sharedSource, feasiblePath bool) {
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
					sharedSource = true
				}
				if ssaflow.ValuesShareErrorSource(argument, reachingReturnValue(returned, result)) {
					feasiblePath = true
				}
			}
		}
	}
	return sharedSource, feasiblePath
}

// Functions containing a defer use shared SSA return slots. Looking at every
// store to such a slot can combine a log branch that stores nil with a
// mutually exclusive error-return branch. Use the store in this concrete
// return block instead. This pattern occurs in CertMagic:
// https://github.com/caddyserver/certmagic/blob/ff600dc62b9bbfc6ba8f18784a2b79000c5e4c75/solvers.go#L794-L800
func reachingReturnValue(returned *ssa.Return, result ssa.Value) ssa.Value {
	load, ok := result.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return result
	}
	for index := ssaflow.InstructionIndex(load) - 1; index >= 0; index-- {
		store, ok := returned.Block().Instrs[index].(*ssa.Store)
		if ok && ssaflow.SameValue(store.Addr, load.X) {
			return store.Val
		}
	}
	return result
}

func traceLogReturnDecision(pass *analysis.Pass, call *ssa.Call, reason string, outcome analysisTrace.Outcome) {
	if !analysisTrace.Enabled("errorownership", string(check.ErrorLogAndReturn)) {
		return
	}
	analysisTrace.Emit(pass, analysisTrace.Event{
		Analyzer: "errorownership", Check: string(check.ErrorLogAndReturn), Phase: "evidence", Reason: reason, Outcome: outcome,
		Pos: call.Pos(), Function: call.Parent().String(),
	})
}
