package cancellationownership

import (
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/ssa"
)

// Cancellation labels. The classifier labels each instruction after the
// constructor with what it does to the cancel function, and names the rule
// that decided the label, so a trace of an unknown says which boundary made
// the use opaque: a launched helper, a registered callback, a store.

type cancellationLabel struct {
	action cancellationAction
	reason cancellationReason
}

func labelled(action cancellationAction, reason cancellationReason) cancellationLabel {
	return cancellationLabel{action: action, reason: reason}
}

// opaqueReference names a use that no rule recognized by what the
// instruction does with the cancel function.
func opaqueReference(instruction ssa.Instruction) cancellationReason {
	switch instruction.(type) {
	case *ssa.Store:
		return reasonLabelStored
	case *ssa.Send:
		return reasonLabelSent
	case *ssa.MapUpdate:
		return reasonLabelStoredInMap
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return reasonLabelPassedToCallee
	case *ssa.Return:
		return reasonLabelReturned
	case *ssa.Phi, *ssa.ChangeType, *ssa.Convert, *ssa.MakeInterface, *ssa.ChangeInterface, *ssa.Extract:
		// The cancel function flows on as another value the classifier does
		// not follow.
		return reasonLabelAliased
	}
	return reasonLabelOpaqueUse
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
