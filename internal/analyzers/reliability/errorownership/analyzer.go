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
				proof := loggedErrorReturnEvidence(call)
				if proof.reason == "" {
					continue
				}
				traceLogReturnDecision(pass, call, proof.reason, proof.outcome)
				if !proof.report {
					continue
				}
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

type logReturnProof struct {
	report  bool
	reason  string
	outcome analysisTrace.Outcome
}

func loggedErrorReturnEvidence(call *ssa.Call) logReturnProof {
	var logged []ssa.Value
	for _, argument := range call.Common().Args {
		if len(ssaflow.ValueSources(argument)) > 0 {
			logged = append(logged, argument)
		}
	}
	if len(logged) == 0 {
		return logReturnProof{}
	}
	sharedSource := false
	hasErrorReturn := false
	for _, returned := range ssaflow.ReachableReturns(call) {
		for _, result := range returned.Results {
			if !syntax.IsErrorType(result.Type()) {
				continue
			}
			hasErrorReturn = true
			for _, argument := range logged {
				if ssaflow.ValuesShareErrorSource(argument, result) {
					sharedSource = true
				}
				if ssaflow.ValuesShareErrorSource(argument, reachingReturnValue(result)) {
					return logReturnProof{report: true, reason: "feasible-log-return-path", outcome: analysisTrace.OutcomeObserved}
				}
			}
		}
	}
	if sharedSource {
		return logReturnProof{reason: "no-feasible-log-return-path", outcome: analysisTrace.OutcomeAccepted}
	}
	if hasErrorReturn {
		return logReturnProof{reason: "independent-error-results", outcome: analysisTrace.OutcomeAccepted}
	}
	return logReturnProof{}
}

// Functions containing a defer use shared SSA return slots. Looking at every
// store to such a slot can combine a log branch that stores nil with a
// mutually exclusive error-return branch. Use the store in this concrete
// return block instead. This pattern occurs in CertMagic:
// https://github.com/caddyserver/certmagic/blob/ff600dc62b9bbfc6ba8f18784a2b79000c5e4c75/solvers.go#L794-L800
func reachingReturnValue(result ssa.Value) ssa.Value {
	load, ok := result.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return result
	}
	// ReachableReturns may expose a load produced in a predecessor block. Search
	// the load's own block: its instruction index has no meaning in the concrete
	// return block, and indexing that unrelated block can panic. EcoHub's
	// repository control flow exercises this shape:
	// https://github.com/fe-spark/EcoHub/tree/f7baa3e7c1978586965d4c1416667c3e9b924597/server/internal/repository
	for index := ssaflow.InstructionIndex(load) - 1; index >= 0; index-- {
		store, ok := load.Block().Instrs[index].(*ssa.Store)
		if ok && ssaflow.SameValue(store.Addr, load.X) {
			return store.Val
		}
	}
	return result
}

func traceLogReturnDecision(pass *analysis.Pass, call *ssa.Call, reason string, outcome analysisTrace.Outcome) {
	analysisTrace.For(pass, "errorownership", string(check.ErrorLogAndReturn), call.Pos()).Evidence(analysisTrace.Step{
		Reason:   reason,
		Outcome:  outcome,
		Pos:      call.Pos(),
		Function: call.Parent().String(),
	})
}
