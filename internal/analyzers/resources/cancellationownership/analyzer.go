// Package cancellationownership implements the cancellationownership gohawk analyzer.
package cancellationownership

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

var summaryKnowledge = summaries.Select(summaries.Requirements{Results: true, Lifecycle: true})

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "cancellationownership",
		Doc:      "checks context and signal-derived cancellation functions proved lost on a normal return path",
		Requires: summaryKnowledge.Requires(),
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
				evidence, _ := summaryKnowledge.Provider(pass).LifecycleEvidence("cancellationownership", string(check.CancellationRelease))
				evidence.ForCandidate(call.Pos())
				proof := proveCancellation(call, cancel, probe, evidence, summaryKnowledge.Provider(pass))
				emitCancellationDecision(pass, function, call, contract, proof)
				// A lost cancel is reported at the call that created it, citing
				// the return the proof reached without calling it.
				if proof.Outcome == CancellationLost {
					source := syntax.SourceRange(pass, call.Pos())
					check.Report(pass, check.CancellationRelease, analysis.Diagnostic{
						Pos: source.Pos(),
						End: source.End(),
						Message: "cancel function from " + syntax.ShortPackageName(
							contract.packagePath,
						) + "." + contract.name + " is not called on every return path",
						Related: check.ReturnEvidence(pass, call, proof.Witness, "calling "+cancelSubject(pass, call, contract.result)),
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
		Reason:   proof.Reason.String(),
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

// cancelSubject names the cancel function by the variable it was assigned to,
// such as `cancel`, when there is one.
func cancelSubject(pass *analysis.Pass, call *ssa.Call, result int) string {
	if name := syntax.AssignedName(pass, call.Pos(), result); name != "" {
		return "`" + name + "`"
	}
	return "the cancel function"
}

// labelReason names a label for the trace.
func (action cancellationAction) labelReason() cancellationReason {
	switch action {
	case cancellationActionRelease:
		return reasonLabelRelease
	case cancellationActionTransfer:
		return reasonLabelTransfer
	case cancellationActionUnknown:
		return reasonLabelOpaqueUse
	case cancellationActionNone:
	}
	return reasonCancellationNone
}

// traceLabel records a release, transfer, or unknown label once, when the
// instruction is first classified. An instruction labelled none is not traced.
func (classifier *cancellationClassifier) traceLabel(instruction ssa.Instruction, action cancellationAction, reason cancellationReason) {
	if action == cancellationActionNone || !classifier.probe.Enabled() {
		return
	}
	outcome := analysisTrace.OutcomeAccepted
	if action == cancellationActionUnknown {
		outcome = analysisTrace.OutcomeUnknown
	}
	classifier.probe.Label(analysisTrace.Step{
		Reason: reason.String(), Outcome: outcome, Pos: instruction.Pos(), Function: instruction.Parent().String(),
		Details: map[string]string{"instruction": instruction.String()},
	})
}
