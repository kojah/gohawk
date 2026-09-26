package cancellationownership

import (
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"golang.org/x/tools/go/ssa"
)

// summaryInvokes reports whether the callee's summary proves it calls this
// exact cancel function before it returns: on every return, or in a case
// the call's constant arguments select. It is asked only once the callee's
// body could not decide, and only a proof counts. A summary that does not
// claim the call proves nothing either way: summary cases carry positive
// guarantees only, so a helper that may still cancel stays unknown rather
// than becoming a leak.
func (classifier *cancellationClassifier) summaryInvokes(instruction ssa.Instruction, request lifecycle.CompletionRequest) bool {
	if classifier.evidence == nil {
		return false
	}
	return classifier.evidence.Prove(lifecyclefacts.EvidenceRequest{
		Instruction: instruction, Target: classifier.cancel, Completion: &request,
		SelectMask: func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask { return fact.SynchronouslyInvoked() },
	}).Proven()
}

// Returned callbacks release this obligation only when their exact factory
// relation guarantees synchronous invocation of this cancellation handle.
func (classifier *cancellationClassifier) returnedCallbackCancels(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if common == nil || common.StaticCallee() != nil || common.IsInvoke() {
		return false
	}
	request := lifecycle.CompletionRequest{
		Instruction: instruction, Target: classifier.cancel, InvokeTarget: true, Budget: classifier.budget(),
	}
	if classifier.evidence == nil {
		return lifecycle.ProveCompletion(request).Proven()
	}
	return classifier.evidence.Prove(lifecyclefacts.EvidenceRequest{
		Instruction: instruction, Target: classifier.cancel, Completion: &request,
	}).Proven()
}
