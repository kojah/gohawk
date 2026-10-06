package processownership

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// This file owns possible Wait participation. Positive captures, merged
// receivers and nonreturning waiters leave ownership uncertain; none proves
// exact reaping. Discovery and completion draw on the supplied allowance.

// A callback supplied to an opaque runner may own the wait. This is a reason
// to decline loss, not evidence that the runner invokes or joins the callback.
// A started worker with no normal return also remains opaque when it contains
// a positive Wait witness: every-return completion deliberately excludes it.
// https://github.com/la5nta/pat/blob/2e6a8d14baf0268f4e2aa4d01784a54ca935cf52/internal/prehook/prehook.go#L109-L114
func provePossibleWaitHandoff(instruction ssa.Instruction, command ssa.Value, budget *proofs.SearchBudget) proofs.Proof {
	missing := proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return missing
	}
	// A merged receiver may select the successfully started command. The
	// flow does not retain acquisition-error/receiver correlation, so possible
	// identity makes this action unknown, never a guaranteed Wait. An earlier
	// return that bypasses the action is still checked by the ordinary flow.
	// https://github.com/raskrebs/sonar/blob/9c963b8447d6ca08dd4a3c0bc6c0bf27527cd793/internal/runs/runs_test.go#L117-L130
	if ssacall.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os/exec", Receiver: "Cmd", Name: "Wait"})) {
		receiver := ssaflow.CallReceiver(common)
		_, merged := receiver.(*ssa.Phi)
		if merged && heapmodel.MayAlias(receiver, command) && !heapmodel.NewStorage(nil).Same(receiver, command).Proven() {
			return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnknownPointee}
		}
		return missing
	}
	callee, _ := ssacall.DirectCallee(common)
	if _, spawned := instruction.(*ssa.Go); spawned && callee != nil && len(callee.Blocks) != 0 {
		returns := ssapath.ProveNormalReturnWithin(callee.Blocks[0], nil, budget)
		if returns.Reason == proofs.EvidenceBudgetExhausted {
			return returns.Proof
		}
		if returns.Proven() {
			return missing
		}
		completion := lifecycle.ProveCompletion(lifecycle.CompletionRequest{
			Instruction: instruction, Target: command, Methods: []string{"Wait"},
			Coverage: lifecycle.CoverageAnywhere, Budget: budget,
		})
		if completion.Proven() || completion.Reason == proofs.EvidenceBudgetExhausted {
			completion.State = proofs.EvidenceUnknown
			return completion.Proof
		}
		return missing
	}
	if callee != nil && len(callee.Blocks) != 0 {
		return missing
	}
	// Imported and unresolved runners can retain the same callback whether
	// called synchronously or launched. The launch does not make their missing
	// invocation guarantee evidence that the captured command stays local.
	// https://github.com/unstablebuild/rune/blob/3e2165f8983280542c985947378dfa740a397d03/internal/workspace/file_scheme.go#L458-L467
	if result := provePossibleCallbackCapture(common.Value, command, budget); result.State == proofs.EvidenceUnknown {
		return result
	}
	for _, argument := range common.Args {
		if result := provePossibleCallbackCapture(argument, command, budget); result.State == proofs.EvidenceUnknown {
			return result
		}
	}
	return missing
}

// A possible capture makes Wait participation unknown. The shared query proves
// only the structural capture, never invocation, joining or exact reaping.
func provePossibleCallbackCapture(value, command ssa.Value, budget *proofs.SearchBudget) proofs.Proof {
	proof := lifecycle.ProvePossibleClosureCaptureWithin(value, command, budget)
	if proof.Proven() {
		proof.State = proofs.EvidenceUnknown
	}
	return proof
}
