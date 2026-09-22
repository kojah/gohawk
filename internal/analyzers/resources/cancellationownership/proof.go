package cancellationownership

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

// CancellationOutcome records the only four conclusions the analyzer may
// draw. In particular, Unknown is not a weaker Lost: ambiguous handoffs
// suppress the default correctness diagnostic.
type CancellationOutcome uint8

const (
	CancellationUnknown CancellationOutcome = iota
	CancellationReleased
	CancellationTransferred
	CancellationLost
)

type cancellationReason string

const (
	reasonCancellationUnknown     cancellationReason = "ambiguous-cancellation-use"
	reasonCancellationReleased    cancellationReason = "exact-cancellation-release"
	reasonCancellationTransferred cancellationReason = "exact-cancellation-transfer"
	reasonCancellationLost        cancellationReason = "unowned-return"
)

// CancellationProof is the authoritative cancellationownership decision.
type CancellationProof struct {
	Outcome CancellationOutcome
	Reason  cancellationReason
}

type cancellationAction uint8

const (
	cancellationActionNone cancellationAction = iota
	cancellationActionRelease
	cancellationActionTransfer
	cancellationActionUnknown
)

type cancellationClassifier struct {
	cancel    ssa.Value
	context   ssa.Value
	parent    *cancellationClassifier
	actions   map[ssa.Instruction]cancellationAction
	transfers bool
	// observer hears where the shared storage, effect, and completion queries
	// behind this proof gave up; nil when the candidate is not being traced.
	observer ssaflow.Observer
}

// Exhausted helper searches remain unknown, never evidence of lost cleanup.
const cancellationCompletionBudget = 1000

func (classifier *cancellationClassifier) budget() *ssaflow.SearchBudget {
	return ssaflow.NewSearchBudget(cancellationCompletionBudget).Observed(classifier.observer)
}

func proveCancellation(call *ssa.Call, cancel ssa.Value, observer ssaflow.Observer) CancellationProof {
	classifier := &cancellationClassifier{
		cancel:   cancel,
		parent:   parentCancellationClassifier(call, observer),
		actions:  make(map[ssa.Instruction]cancellationAction),
		observer: observer,
	}
	if contract, ok := cancellationContractFor(call.Common()); ok && contract.packagePath == "context" {
		classifier.context = ssaflow.CallResult(call, 0)
	}
	// One walk carries the classifier's labels to every feasible return. A
	// return no action reaches is loss; a return only an opaque handoff reaches
	// is unknown, and that opacity excuses no other path's early return.
	switch ssaflow.EvaluateObligation(ssaflow.ObligationFlow{
		Start: call, NonNil: cancel,
		Instruction: classifier.obligation, Return: classifier.returnObligation, Edge: classifier.edgeObligation,
	}) {
	case ssaflow.ObligationViolated:
		return CancellationProof{Outcome: CancellationLost, Reason: reasonCancellationLost}
	case ssaflow.ObligationUncertain:
		return CancellationProof{Outcome: CancellationUnknown, Reason: reasonCancellationUnknown}
	case ssaflow.ObligationHonored:
	}
	if classifier.transfers {
		return CancellationProof{Outcome: CancellationTransferred, Reason: reasonCancellationTransferred}
	}
	return CancellationProof{Outcome: CancellationReleased, Reason: reasonCancellationReleased}
}

// obligation, returnObligation, and edgeObligation map this classifier's
// labels onto the shared flow lattice: a release or transfer is exact
// evidence, an ambiguous use is opaque, and a selected Done receive is an
// edge-local opaque observation of cancellation.
func (classifier *cancellationClassifier) obligation(instruction ssa.Instruction) ssaflow.ObligationAction {
	return cancellationObligation(classifier.action(instruction))
}

func (classifier *cancellationClassifier) returnObligation(returned *ssa.Return) ssaflow.ObligationAction {
	return cancellationObligation(classifier.returnAction(returned))
}

func (classifier *cancellationClassifier) edgeObligation(from, to *ssa.BasicBlock) ssaflow.ObligationAction {
	if ssaflow.ProveCompletionOnEdge(from, to, ssaflow.CompletionRequest{
		Target: classifier.cancel, InvokeTarget: true, Budget: classifier.budget(),
	}).Proven() {
		return ssaflow.ObligationExact
	}
	if classifier.selectedDoneEdge(from, to) {
		return ssaflow.ObligationUnknown
	}
	return ssaflow.ObligationNone
}

func cancellationObligation(action cancellationAction) ssaflow.ObligationAction {
	switch action {
	case cancellationActionRelease, cancellationActionTransfer:
		return ssaflow.ObligationExact
	case cancellationActionUnknown:
		return ssaflow.ObligationUnknown
	case cancellationActionNone:
	}
	return ssaflow.ObligationNone
}

func (classifier *cancellationClassifier) action(instruction ssa.Instruction) cancellationAction {
	if action, ok := classifier.actions[instruction]; ok {
		return action
	}
	action := classifier.classifyAction(instruction)
	if action == cancellationActionNone && classifier.parent != nil && classifier.parent.action(instruction) != cancellationActionNone {
		action = cancellationActionUnknown
	}
	classifier.actions[instruction] = action
	if action == cancellationActionTransfer {
		classifier.transfers = true
	}
	return action
}

// A fresh standard context child also follows its exact parent's cancellation.
// Reuse the existing classifier for that alternate owner, but only as unknown:
// an opaque handoff of the parent is not proof of when its child is canceled.
// Signal registrations still require their own stop function; cancellation of
// their parent does not unregister them. Wrapped and merged parents stay opaque.
// https://github.com/crazy-max/diun/blob/269cb27295944aeacfe549d24ab7ac483e600aa9/internal/notif/apprise/client.go#L91-L94
func parentCancellationClassifier(call *ssa.Call, observer ssaflow.Observer) *cancellationClassifier {
	contract, ok := cancellationContractFor(call.Common())
	if !ok || contract.packagePath != "context" || len(call.Common().Args) == 0 {
		return nil
	}
	// Contexts captured by workers are held in local cells. Resolve the load
	// where this child is created, not at the later cancellation or return: the
	// same source variable may subsequently hold the child instead of its parent.
	// https://github.com/werf/nelm/blob/6393382d695e65d8d8f744cf590337fe62a83eef/pkg/action/release_install.go#L179-L190
	parentValue := call.Common().Args[0]
	if resolved := ssaflow.NewStorage(ssaflow.NewSearchBudget(cancellationCompletionBudget).Observed(observer)).Resolve(parentValue); resolved.Proven() {
		parentValue = resolved.Value
	}
	parent, ok := parentValue.(*ssa.Extract)
	if !ok || parent.Index != 0 {
		return nil
	}
	constructor, ok := parent.Tuple.(*ssa.Call)
	if !ok || constructor.Parent() != call.Parent() {
		return nil
	}
	parentContract, ok := cancellationContractFor(constructor.Common())
	if !ok {
		return nil
	}
	cancel := ssaflow.CallResult(constructor, parentContract.result)
	if cancel == nil {
		return nil
	}
	return &cancellationClassifier{cancel: cancel, actions: make(map[ssa.Instruction]cancellationAction), observer: observer}
}

func (classifier *cancellationClassifier) classifyAction(instruction ssa.Instruction) cancellationAction {
	// Creating a callback does not itself release or transfer cancellation. Its
	// eventual defer, return, store, launch, or opaque call is classified at the
	// instruction that establishes that lifecycle consequence.
	if _, ok := instruction.(*ssa.MakeClosure); ok {
		return cancellationActionNone
	}
	common := ssaflow.InstructionCall(instruction)
	if action, recognized := classifier.recognizedAction(instruction, common); recognized {
		return action
	}
	if !instructionReferencesCancellation(instruction, classifier.cancel) {
		return cancellationActionNone
	}
	if localStorageOnly(instruction) {
		return cancellationActionNone
	}
	if localCallOnlyObserves(instruction, classifier.cancel, classifier.observer) {
		return cancellationActionNone
	}
	return cancellationActionUnknown
}

func (classifier *cancellationClassifier) recognizedAction(
	instruction ssa.Instruction,
	common *ssa.CallCommon,
) (cancellationAction, bool) {
	if action, recognized := classifier.recognizedDirectAction(instruction, common); recognized {
		return action, true
	}
	return classifier.recognizedCallAction(instruction, common)
}

func (classifier *cancellationClassifier) recognizedDirectAction(
	instruction ssa.Instruction,
	common *ssa.CallCommon,
) (cancellationAction, bool) {
	if receive, ok := instruction.(*ssa.UnOp); ok && receive.Op == token.ARROW && classifier.ownDoneChannel(receive.X) {
		return cancellationActionUnknown, true
	}
	if common != nil && common.Value == classifier.cancel {
		if _, ok := instruction.(*ssa.Go); ok {
			return cancellationActionTransfer, true
		}
		return cancellationActionRelease, true
	}
	// Captured callbacks and helper chains are deliberately ambiguous here.
	// These broad closure traversals are safe for finding a possible handoff,
	// but not exact enough to prove which callback executes on every path.
	if ssaflow.DeferredClosureCallsValue(instruction, classifier.cancel) ||
		ssaflow.DeferredClosureInvokesArgumentOnEveryReturn(instruction, classifier.cancel) ||
		deferredClosureCaptures(instruction, classifier.cancel) {
		return cancellationActionUnknown, true
	}
	if deferredClosureUseIsLocallyResolved(instruction, classifier.cancel, classifier.observer) {
		return cancellationActionNone, true
	}
	return cancellationActionNone, false
}

func (classifier *cancellationClassifier) recognizedCallAction(
	instruction ssa.Instruction,
	common *ssa.CallCommon,
) (cancellationAction, bool) {
	if common != nil && ssaflow.HasLibraryContract(common, ssaflow.ContractTestingCleanup) &&
		commonHasExactArgument(common, classifier.cancel) {
		return cancellationActionTransfer, true
	}
	// Timers and framework registrars do not guarantee that an installed
	// callback runs. They are deliberately left to the Unknown branch even when
	// their API or method name suggests cleanup.
	if common != nil && (ssaflow.HasLibraryContract(common, ssaflow.ContractAfterFunc) ||
		ssaflow.HasLibraryContract(common, ssaflow.ContractDeferredCleanup)) &&
		instructionReferencesCancellation(instruction, classifier.cancel) {
		return cancellationActionUnknown, true
	}
	if common != nil && commonHasExactArgument(common, classifier.cancel) {
		if _, launched := instruction.(*ssa.Go); launched {
			// Passing the exact cancel function to a source-visible helper launched
			// concurrently is an explicit handoff, but conditional invocation inside
			// that worker is not proof of release. Treat it as Unknown so the default
			// check does not turn an event-driven cancellation contract into a leak.
			// https://github.com/infercrane/infercrane/blob/93a43cebe36e01c68c1517d5f1eb97417d01588d/internal/asyncinference/service_lease_test.go#L43-L54
			return cancellationActionUnknown, true
		}
		completion := ssaflow.ProveCompletion(ssaflow.CompletionRequest{
			Instruction: instruction, Target: classifier.cancel, InvokeTarget: true,
			Budget: classifier.budget(),
		})
		switch completion.State {
		case ssaflow.EvidenceProven:
			return cancellationActionRelease, true
		case ssaflow.EvidenceUnknown:
			return cancellationActionUnknown, true
		case ssaflow.EvidenceDisproven:
		}
		// The older may-alias invocation query can still identify an ambiguous
		// handoff outside exact completion's boundary, but cannot prove release.
		if ssaflow.CallInvokesArgumentOnEveryReturn(instruction, classifier.cancel) {
			return cancellationActionUnknown, true
		}
		if ssaflow.CallReturnsDeferredCleanup(instruction, classifier.cancel) {
			return cancellationActionUnknown, true
		}
	}
	if common != nil && slices.ContainsFunc(common.Args, func(argument ssa.Value) bool {
		_, closure := argument.(*ssa.MakeClosure)
		return closure && ssaflow.MayContainValue(argument, classifier.cancel)
	}) {
		// A callback which captures cancel may be invoked, retained, or discarded
		// by the callee. Without an exact callback contract, none of those
		// possibilities establishes loss or release. Vekil passes cancellation
		// through request callbacks whose execution is owned by the helper:
		// https://github.com/sozercan/vekil/blob/842f12f7875143274378fcbb80d411295edf3d28/cmd/menubar/portal_linux_test.go#L210-L230
		return cancellationActionUnknown, true
	}
	return cancellationActionNone, false
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
		return ssaflow.CapturedBindingMatches(binding, target)
	})
}

func (classifier *cancellationClassifier) returnAction(returned *ssa.Return) cancellationAction {
	if slices.Contains(returned.Results, classifier.cancel) {
		classifier.transfers = true
		return cancellationActionTransfer
	}
	if ssaflow.ReturnedValueOwnsValue(returned, classifier.cancel) {
		return cancellationActionUnknown
	}
	if classifier.parent != nil && classifier.parent.returnAction(returned) != cancellationActionNone {
		return cancellationActionUnknown
	}
	return cancellationActionNone
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
		ssaflow.CallMatchesSymbol(call.Common(), syntax.PackageMethod(syntax.MethodSymbol{
			PackagePath: "context", Receiver: "Context", Name: "Done",
		}))
}

func (classifier *cancellationClassifier) selectedDoneEdge(from, to *ssa.BasicBlock) bool {
	if classifier.context == nil {
		return false
	}
	channel, selected := ssaflow.SelectedReceiveOnEdge(from, to)
	return selected && classifier.ownDoneChannel(channel)
}

func commonHasExactArgument(common *ssa.CallCommon, target ssa.Value) bool {
	return slices.Contains(common.Args, target)
}

func instructionReferencesCancellation(instruction ssa.Instruction, cancel ssa.Value) bool {
	for _, operand := range instruction.Operands(nil) {
		if operand == nil || *operand == nil {
			continue
		}
		if *operand == cancel || ssaflow.MayAlias(*operand, cancel) || ssaflow.MayContainValue(*operand, cancel) ||
			addressStoresCancellation(*operand, cancel) {
			return true
		}
	}
	return false
}

// Ambiguous-use detection deliberately follows more aliases than exact
// release proofs. A broad match here can only suppress a diagnostic; it can
// never establish that cancellation was released or transferred.

// cancellationForms are the wrappers a cancel function keeps its identity
// through while the analysis looks for the address it is stored into.
const cancellationForms = ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface

func addressStoresCancellation(value, cancel ssa.Value) bool {
	return ssaflow.NewReachingWalk(cancellationForms).Any(value, func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		return addressStoresCancellationLeaf(walk, value, cancel)
	})
}

func addressStoresCancellationLeaf(walk ssaflow.ReachingWalk, value, cancel ssa.Value) bool {
	if loaded, ok := value.(*ssa.UnOp); ok && walk.Any(loaded.X, func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		return addressStoresCancellationLeaf(walk, value, cancel)
	}) {
		return true
	}
	if value.Referrers() == nil {
		return false
	}
	for _, reference := range *value.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == value && (store.Val == cancel || ssaflow.MayAlias(store.Val, cancel)) {
			return true
		}
	}
	return false
}

func deferredClosureUseIsLocallyResolved(instruction ssa.Instruction, cancel ssa.Value, observer ssaflow.Observer) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	closure, ok := common.Value.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	function := common.StaticCallee()
	if function == nil || len(function.Blocks) == 0 {
		return false
	}
	found := false
	for _, captured := range ssaflow.ClosureBindingPairs(function, closure) {
		if !ssaflow.CapturedBindingMatches(captured.Binding, cancel) {
			continue
		}
		found = true
		if !newCancellationUse(observer).parameterResolved(function, captured.Free) {
			return false
		}
	}
	return found
}

func localCallOnlyObserves(instruction ssa.Instruction, cancel ssa.Value, observer ssaflow.Observer) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) == 0 {
		return false
	}
	callee := common.StaticCallee()
	// Proven read-only use is not cancellation. Unknown effects still go
	// through the cancellation-specific invocation policy below.
	if ssaflow.NewCallEffects(ssaflow.NewSearchBudget(cancellationCompletionBudget).Observed(observer)).Call(instruction, cancel).PreservesStorage() {
		return true
	}
	found := false
	for _, binding := range ssaflow.CallBindings(common, callee, nil) {
		argument := binding.Supplied
		closureContainsCancel := false
		if _, ok := argument.(*ssa.MakeClosure); ok {
			closureContainsCancel = ssaflow.MayContainValue(argument, cancel)
		}
		if argument != cancel && !closureContainsCancel {
			continue
		}
		found = true
		if !newCancellationUse(observer).parameterResolved(callee, binding.Local) {
			return false
		}
	}
	return found
}

// cancellationUse answers whether every use of a cancel value inside a callee
// is resolved. The memo owns the cycle guard and the rule that an answer cut
// short by it is not retained.
type cancellationUse struct {
	memo   *ssaflow.CallGraphMemo[cancellationUseKey, bool]
	budget *ssaflow.SearchBudget
}

type cancellationUseKey struct {
	function  *ssa.Function
	parameter ssa.Value
}

func newCancellationUse(observer ssaflow.Observer) *cancellationUse {
	return &cancellationUse{
		memo:   ssaflow.NewCallGraphMemo[cancellationUseKey, bool](),
		budget: ssaflow.NewSearchBudget(cancellationCompletionBudget).Observed(observer),
	}
}

func (search *cancellationUse) parameterResolved(function *ssa.Function, parameter ssa.Value) bool {
	key := cancellationUseKey{function: function, parameter: parameter}
	return search.memo.Summarize(key, function, search.budget, func() bool {
		return search.searchParameterResolved(function, parameter)
	}, func(ssaflow.SummaryUnavailable, bool) bool {
		// Unresolved use stays an opaque consumption at the classifier. A
		// shortened search must not prove that the helper only observes cancel.
		return false
	})
}

func (search *cancellationUse) searchParameterResolved(function *ssa.Function, parameter ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !search.budget.Spend() {
				return false
			}
			if instructionReferencesCancellation(instruction, parameter) &&
				!search.instructionResolved(instruction, parameter) {
				return false
			}
		}
	}
	return true
}

func (search *cancellationUse) instructionResolved(instruction ssa.Instruction, parameter ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		if _, ok := instruction.(*ssa.DebugRef); ok || localStorageOnly(instruction) {
			return true
		}
		value, ok := instruction.(ssa.Value)
		return ok && exactLocalValueUse(value, parameter)
	}
	if exactLocalValueUse(common.Value, parameter) {
		return true
	}
	callee := common.StaticCallee()
	if callee == nil || len(callee.Blocks) == 0 {
		return false
	}
	matched := false
	for _, binding := range ssaflow.CallBindings(common, callee, nil) {
		if !search.budget.Spend() {
			return false
		}
		if !exactLocalValueUse(binding.Supplied, parameter) {
			continue
		}
		matched = true
		if !search.parameterResolved(callee, binding.Local) {
			return false
		}
	}
	return matched
}

func exactLocalValueUse(value, parameter ssa.Value) bool {
	if value == parameter {
		return true
	}
	if inner, ok := ssaflow.UnwrapTransparentValue(
		value,
		ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		return exactLocalValueUse(inner, parameter)
	}
	loaded, ok := value.(*ssa.UnOp)
	return ok && loaded.Op == token.MUL && exactLocalValueUse(loaded.X, parameter)
}

// localStorageOnly accepts a store into a local that no other code can read.
// A local captured by a closure is not private: a deferred guard such as
// `if cancelWorker != nil { cancelWorker() }` may release the stored cancel
// on every return, and this proof does not follow that closure, so the store
// is an ambiguous handoff rather than plain retention. Safebucket's worker
// lock loop uses exactly that shape:
// https://github.com/safebucket/safebucket/blob/f35560194cb6ea01a4607c2fe36ead2c7db51b9d/internal/core/bootstrap.go#L256-L297
func localStorageOnly(instruction ssa.Instruction) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok {
		return false
	}
	local, ok := store.Addr.(*ssa.Alloc)
	return ok && !ssaflow.ValueEscapes(local) && !capturedByClosure(local)
}

func capturedByClosure(local *ssa.Alloc) bool {
	if local.Referrers() == nil {
		return false
	}
	for _, reference := range *local.Referrers() {
		if _, ok := reference.(*ssa.MakeClosure); ok {
			return true
		}
	}
	return false
}
