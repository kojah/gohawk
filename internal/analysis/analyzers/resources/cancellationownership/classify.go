package cancellationownership

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/syntax"
	"golang.org/x/tools/go/ssa"
)

// Classification labels each instruction once for the cancellation flow.
// Exact invocation and transfer stay distinct from opaque consumption; parent
// context and Done observations supply uncertainty rather than exact cleanup.

func (classifier *cancellationClassifier) action(instruction ssa.Instruction) cancellationAction {
	if action, ok := classifier.actions[instruction]; ok {
		return action
	}
	label := classifier.classifyAction(instruction)
	action, reason := label.action, label.reason
	if action == cancellationActionNone && classifier.parent != nil && classifier.parent.action(instruction) != cancellationActionNone {
		action, reason = cancellationActionUnknown, reasonLabelParentContextUse
	}
	if returned, ok := instruction.(*ssa.Return); ok {
		// Flow can revisit a merged return under different path states. Its
		// result and owner evidence is instruction-local: combine it with the
		// ordinary label once, so neither queries nor trace labels repeat.
		// Exact return cleanup/transfer still covers an opaque ordinary use.
		returnedLabel := classifier.returnLabel(returned)
		if returnedLabel.action != cancellationActionNone && cancellationObligation(returnedLabel.action) >= cancellationObligation(action) {
			action, reason = returnedLabel.action, returnedLabel.reason
		}
	}
	classifier.actions[instruction] = action
	classifier.traceLabel(instruction, action, reason)
	if action == cancellationActionTransfer {
		classifier.transfers = true
	}
	return action
}

func (classifier *cancellationClassifier) classifyAction(instruction ssa.Instruction) cancellationLabel {
	// Creating a callback does not itself release or transfer cancellation. Its
	// eventual defer, return, store, launch, or opaque call is classified at the
	// instruction that establishes that lifecycle consequence.
	if _, ok := instruction.(*ssa.MakeClosure); ok {
		return cancellationLabel{}
	}
	if label, recognized := classifier.deferredCaptureStoreLabel(instruction); recognized {
		return label
	}

	owner := classifier.cancellationOwner()
	if owner.Owner != nil && owner.Owner.holds[instruction] {
		return cancellationLabel{}
	}
	common := ssaflow.InstructionCall(instruction)
	if label, recognized := classifier.recognizedAction(instruction, common); recognized {
		return label
	}
	if !owner.Known() {
		return labelled(cancellationActionUnknown, reasonLabelOwnerUnavailable)
	}
	if _, returned := instruction.(*ssa.Return); returned && classifier.processLifetime {
		// A standard context acquired once by the true program entry may span
		// the process lifetime: its retention cannot accumulate at this site.
		// Exit ends that interval but does not invoke cancel or prove workers
		// joined. Signal registrations and repeatable acquisitions stay outside
		// this boundary. Exact cleanup/transfer can still cover this unknown.
		// https://github.com/k8ssandra/k8ssandra-operator/blob/2028d352ecb495de4b6e053d99d7a77b21eb5107/main.go#L176-L205
		return labelled(cancellationActionUnknown, reasonLabelProcessLifetimeContext)
	}
	if !instructionReferencesCancellation(instruction, classifier.cancel) {
		return cancellationLabel{}
	}
	if localStorageOnly(instruction) {
		return cancellationLabel{}
	}
	if localCallOnlyObserves(instruction, classifier.cancel, classifier.observer) {
		return cancellationLabel{}
	}
	return labelled(cancellationActionUnknown, opaqueReference(instruction))
}

func (classifier *cancellationClassifier) recognizedAction(
	instruction ssa.Instruction,
	common *ssa.CallCommon,
) (cancellationLabel, bool) {
	if label, recognized := classifier.recognizedDirectAction(instruction, common); recognized {
		return label, true
	}
	return classifier.recognizedCallAction(instruction, common)
}

func (classifier *cancellationClassifier) recognizedDirectAction(
	instruction ssa.Instruction,
	common *ssa.CallCommon,
) (cancellationLabel, bool) {
	if receive, ok := instruction.(*ssa.UnOp); ok && receive.Op == token.ARROW && classifier.ownDoneChannel(receive.X) {
		return labelled(cancellationActionUnknown, reasonLabelOwnDoneReceive), true
	}
	if common != nil && common.Value == classifier.cancel {
		if _, ok := instruction.(*ssa.Go); ok {
			return labelled(cancellationActionTransfer, reasonLabelLaunchedCancel), true
		}
		return labelled(cancellationActionRelease, reasonLabelRelease), true
	}
	if deferred, ok := instruction.(*ssa.Defer); ok {
		if label, decided := classifier.deferredLiteralLabel(deferred); decided {
			return label, true
		}
	}
	// Captured callbacks and helper chains are deliberately ambiguous here.
	// These broad closure traversals are safe for finding a possible handoff,
	// but not exact enough to prove which callback executes on every path.
	if lifecycle.DeferredClosureCallsValue(instruction, classifier.cancel) ||
		lifecycle.DeferredClosureInvokesArgumentOnEveryReturn(instruction, classifier.cancel) ||
		deferredClosureCaptures(instruction, classifier.cancel) {
		return labelled(cancellationActionUnknown, reasonLabelDeferredClosure), true
	}
	if deferredClosureUseIsLocallyResolved(instruction, classifier.cancel, classifier.observer) {
		return cancellationLabel{}, true
	}
	return cancellationLabel{}, false
}

func (classifier *cancellationClassifier) recognizedCallAction(
	instruction ssa.Instruction,
	common *ssa.CallCommon,
) (cancellationLabel, bool) {
	if classifier.returnedCallbackCancels(instruction, common) {
		return labelled(cancellationActionRelease, reasonLabelReturnedCallback), true
	}
	if common != nil && ssacall.HasLibraryContract(common, ssacall.ContractTestingCleanup) &&
		commonHasExactArgument(common, classifier.cancel) {
		return labelled(cancellationActionTransfer, reasonLabelTestingCleanup), true
	}
	// Timers and framework registrars do not guarantee that an installed
	// callback runs. They are deliberately left to the Unknown branch even when
	// their API or method name suggests cleanup.
	if common != nil && (ssacall.HasLibraryContract(common, ssacall.ContractAfterFunc) ||
		ssacall.HasLibraryContract(common, ssacall.ContractDeferredCleanup)) &&
		instructionReferencesCancellation(instruction, classifier.cancel) {
		return labelled(cancellationActionUnknown, reasonLabelRegisteredCallback), true
	}
	if common != nil && commonHasExactArgument(common, classifier.cancel) {
		if label, recognized := classifier.exactArgumentAction(instruction); recognized {
			return label, true
		}
	}
	if common != nil && slices.ContainsFunc(common.Args, func(argument ssa.Value) bool {
		_, closure := argument.(*ssa.MakeClosure)
		return closure && lifecycle.MayContainValue(argument, classifier.cancel)
	}) {
		// A callback which captures cancel may be invoked, retained, or discarded
		// by the callee. Without an exact callback contract, none of those
		// possibilities establishes loss or release. Vekil passes cancellation
		// through request callbacks whose execution is owned by the helper:
		// https://github.com/sozercan/vekil/blob/842f12f7875143274378fcbb80d411295edf3d28/cmd/menubar/portal_linux_test.go#L210-L230
		return labelled(cancellationActionUnknown, reasonLabelCapturedByCallback), true
	}
	return cancellationLabel{}, false
}

// exactArgumentAction labels a call that passes the exact cancel function
// as an argument: launched, proven called, or undecided.
func (classifier *cancellationClassifier) exactArgumentAction(instruction ssa.Instruction) (cancellationLabel, bool) {
	if _, launched := instruction.(*ssa.Go); launched {
		// Passing the exact cancel function to a source-visible helper launched
		// concurrently is an explicit handoff, but conditional invocation inside
		// that worker is not proof of release. Treat it as Unknown so the default
		// check does not turn an event-driven cancellation contract into a leak.
		// https://github.com/infercrane/infercrane/blob/93a43cebe36e01c68c1517d5f1eb97417d01588d/internal/asyncinference/service_lease_test.go#L43-L54
		return labelled(cancellationActionUnknown, reasonLabelLaunchedHelper), true
	}
	request := lifecycle.CompletionRequest{
		Instruction: instruction, Target: classifier.cancel, InvokeTarget: true,
		Budget: classifier.budget(),
	}
	completion := lifecycle.ProveCompletion(request)
	switch completion.State {
	case proofs.EvidenceProven:
		return labelled(cancellationActionRelease, reasonLabelHelperRelease), true
	case proofs.EvidenceUnknown:
		if classifier.summaryInvokes(instruction, request) {
			return labelled(cancellationActionRelease, reasonLabelSummaryRelease), true
		}
		return labelled(cancellationActionUnknown, reasonLabelHelperUndecided), true
	case proofs.EvidenceDisproven:
	}
	// The older may-alias invocation query can still identify an ambiguous
	// handoff outside exact completion's boundary, but cannot prove release.
	if lifecycle.CallInvokesArgumentOnEveryReturn(instruction, classifier.cancel) {
		return labelled(cancellationActionUnknown, reasonLabelHelperMayInvoke), true
	}
	if lifecycle.CallReturnsDeferredCleanup(instruction, classifier.cancel) {
		return labelled(cancellationActionUnknown, reasonLabelReturnsDeferredCleanup), true
	}
	return cancellationLabel{}, false
}

func deferredClosureCaptures(instruction ssa.Instruction, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	closure, ok := common.Value.(*ssa.MakeClosure)
	return ok && slices.ContainsFunc(closure.Bindings, func(binding ssa.Value) bool {
		return heapmodel.CapturedBindingMatches(binding, target)
	})
}

func (classifier *cancellationClassifier) returnLabel(returned *ssa.Return) cancellationLabel {
	if label, decided := classifier.resultGuardedReturn(returned); decided && !slices.Contains(returned.Results, classifier.cancel) {
		return label
	}
	if slices.Contains(returned.Results, classifier.cancel) {
		return labelled(cancellationActionTransfer, reasonLabelReturned)
	}
	if label := classifier.ownerReturnLabel(returned); label.action != cancellationActionNone {
		return label
	}
	if lifecycle.ReturnedValueOwnsValue(returned, classifier.cancel) {
		return labelled(cancellationActionUnknown, reasonLabelReturned)
	}
	return cancellationLabel{}
}

// Receiving this standard context's Done observes cancellation, not just an
// intention to cancel. Keep it unknown rather than claiming synchronous parent
// unlinking. NotifyContext is excluded: receiving a signal does not unregister
// its handler. Each select arm must be proved separately; another case or a
// default arm must not borrow this evidence.
// https://github.com/chrislusf/gleam/blob/8b4ae277059f30322db71de5e0473a4351bf9f8d/util/context.go#L7-L25
func (classifier *cancellationClassifier) ownDoneChannel(value ssa.Value) bool {
	call, ok := value.(*ssa.Call)
	return ok && classifier.context != nil && ssaflow.CallReceiver(call.Common()) == classifier.context &&
		ssacall.CallMatchesSymbol(call.Common(), syntax.PackageMethod(syntax.MethodSymbol{
			PackagePath: "context", Receiver: "Context", Name: "Done",
		}))
}

func (classifier *cancellationClassifier) selectedDoneEdge(from, to *ssa.BasicBlock) bool {
	if classifier.context == nil {
		return false
	}
	channel, selected := ssapath.SelectedReceiveOnEdge(from, to)
	return selected && classifier.ownDoneChannel(channel)
}

func commonHasExactArgument(common *ssa.CallCommon, target ssa.Value) bool {
	return slices.Contains(common.Args, target)
}
