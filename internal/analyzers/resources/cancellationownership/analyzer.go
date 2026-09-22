// Package cancellationownership implements the cancellationownership gohawk analyzer.
package cancellationownership

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
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
		Name:     "cancellationownership",
		Doc:      "checks context and signal-derived cancellation functions proved lost on a normal return path",
		Requires: []*analysis.Analyzer{buildssa.Analyzer, lifecyclefacts.Analyzer},
		Run:      runCancellationOwnership,
	}
}

func runCancellationOwnership(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok {
					continue
				}
				contract, ok := cancellationContractFor(call.Common())
				if !ok {
					continue
				}
				cancel := ssaflow.CallResult(call, contract.result)
				if cancel == nil {
					continue
				}
				probe := analysisTrace.For(pass, "cancellationownership", string(check.CancellationRelease), call.Pos())
				evidence := lifecyclefacts.NewLifecycleEvidence(pass, "cancellationownership", string(check.CancellationRelease))
				evidence.ForCandidate(call.Pos())
				proof := proveCancellation(call, cancel, probe.Observer(), evidence)
				emitCancellationDecision(pass, function, call, contract, proof)
				if proof.Outcome == CancellationLost {
					source := syntax.SourceRange(pass, call.Pos())
					check.Report(pass, check.CancellationRelease, analysis.Diagnostic{
						Pos: source.Pos(),
						End: source.End(),
						Message: "cancel function from " + syntax.ShortPackageName(
							contract.packagePath,
						) + "." + contract.name + " is not called on every return path",
					})
				}
			}
		}
	}
	return nil, nil
}

func emitCancellationDecision(
	pass *analysis.Pass,
	function *ssa.Function,
	call *ssa.Call,
	contract cancellationContract,
	proof CancellationProof,
) {
	checkID := string(check.CancellationRelease)
	if !analysisTrace.Enabled("cancellationownership", checkID) {
		return
	}
	outcome := analysisTrace.OutcomeAccepted
	switch proof.Outcome {
	case CancellationLost:
		outcome = analysisTrace.OutcomeRejected
	case CancellationUnknown:
		outcome = analysisTrace.OutcomeUnknown
	case CancellationReleased, CancellationTransferred:
	}
	analysisTrace.For(pass, "cancellationownership", checkID, call.Pos()).Decision(analysisTrace.Step{
		Reason:   string(proof.Reason),
		Outcome:  outcome,
		Pos:      call.Pos(),
		Function: function.String(),
		Details:  map[string]string{"constructor": contract.packagePath + "." + contract.name},
	})
}

// Constructor contracts are the source of the cancellation obligation. Keep
// this list limited to standard APIs whose second result is documented as a
// release function; project wrappers remain ordinary, potentially ambiguous
// data flow handled by the proof layer.

type cancellationContract struct {
	symbol      syntax.Symbol
	packagePath string
	name        string
	result      int
}

var cancellationContracts = []cancellationContract{
	cancellationFunction("context", "WithCancel"),
	cancellationFunction("context", "WithCancelCause"),
	cancellationFunction("context", "WithDeadline"),
	cancellationFunction("context", "WithDeadlineCause"),
	cancellationFunction("context", "WithTimeout"),
	cancellationFunction("context", "WithTimeoutCause"),
	cancellationFunction("os/signal", "NotifyContext"),
}

func cancellationFunction(packagePath, name string) cancellationContract {
	return cancellationContract{symbol: syntax.PackageFunction(packagePath, name), packagePath: packagePath, name: name, result: 1}
}

func cancellationContractFor(common *ssa.CallCommon) (cancellationContract, bool) {
	for _, contract := range cancellationContracts {
		if ssaflow.CallMatchesSymbol(common, contract.symbol) {
			return contract, true
		}
	}
	return cancellationContract{}, false
}
