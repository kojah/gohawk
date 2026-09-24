package cancellationownership

import (
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssainfer"
	"golang.org/x/tools/go/ssa"
)

// Returned callbacks release this obligation only when their exact factory
// relation guarantees synchronous invocation of this cancellation handle.
func (classifier *cancellationClassifier) returnedCallbackCancels(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if common == nil || common.StaticCallee() != nil || common.IsInvoke() {
		return false
	}
	request := ssainfer.CompletionRequest{
		Instruction: instruction, Target: classifier.cancel, InvokeTarget: true, Budget: classifier.budget(),
	}
	if classifier.evidence == nil {
		return ssainfer.ProveCompletion(request).Proven()
	}
	return classifier.evidence.Prove(lifecyclefacts.EvidenceRequest{
		Instruction: instruction, Target: classifier.cancel, Completion: &request,
	}).Proven()
}
