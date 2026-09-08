package cancellationownership

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"

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
	actions   map[ssa.Instruction]cancellationAction
	transfers bool
}

func proveCancellation(call *ssa.Call, cancel ssa.Value) CancellationProof {
	classifier := &cancellationClassifier{
		cancel:  cancel,
		actions: make(map[ssa.Instruction]cancellationAction),
	}
	settledOrUnknown := func(instruction ssa.Instruction) bool {
		return classifier.action(instruction) != cancellationActionNone
	}
	returnSettledOrUnknown := func(returned *ssa.Return) bool {
		return classifier.returnAction(returned) != cancellationActionNone
	}
	if ssaflow.UnownedReturnAssumingNonNil(call, cancel, settledOrUnknown, returnSettledOrUnknown) {
		return CancellationProof{Outcome: CancellationLost, Reason: reasonCancellationLost}
	}
	exact := func(instruction ssa.Instruction) bool {
		action := classifier.action(instruction)
		return action == cancellationActionRelease || action == cancellationActionTransfer
	}
	returnExact := func(returned *ssa.Return) bool {
		return classifier.returnAction(returned) == cancellationActionTransfer
	}
	if ssaflow.UnownedReturnAssumingNonNil(call, cancel, exact, returnExact) {
		return CancellationProof{Outcome: CancellationUnknown, Reason: reasonCancellationUnknown}
	}
	if classifier.transfers {
		return CancellationProof{Outcome: CancellationTransferred, Reason: reasonCancellationTransferred}
	}
	return CancellationProof{Outcome: CancellationReleased, Reason: reasonCancellationReleased}
}

func (classifier *cancellationClassifier) action(instruction ssa.Instruction) cancellationAction {
	if action, ok := classifier.actions[instruction]; ok {
		return action
	}
	action := classifier.classifyAction(instruction)
	classifier.actions[instruction] = action
	if action == cancellationActionTransfer {
		classifier.transfers = true
	}
	return action
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
	if localCallOnlyObserves(instruction, classifier.cancel) {
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
	if deferredClosureUseIsLocallyResolved(instruction, classifier.cancel) {
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
		if callDirectlyInvokesExactArgumentOnEveryReturn(instruction, classifier.cancel) {
			return cancellationActionRelease, true
		}
		// A nested static helper may settle the callback, but the shared helper
		// traversal intentionally accepts aliases that are too broad for an
		// exact release proof. Preserve it only as conservative Unknown evidence.
		if ssaflow.CallInvokesArgumentOnEveryReturn(instruction, classifier.cancel) {
			return cancellationActionUnknown, true
		}
		if ssaflow.CallReturnsDeferredCleanup(instruction, classifier.cancel) {
			return cancellationActionUnknown, true
		}
	}
	if common != nil && slices.ContainsFunc(common.Args, func(argument ssa.Value) bool {
		_, closure := argument.(*ssa.MakeClosure)
		return closure && ssaflow.ValueContainsValue(argument, classifier.cancel)
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

func callDirectlyInvokesExactArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) == 0 {
		return false
	}
	callee := common.StaticCallee()
	for index, argument := range common.Args {
		if argument != target || index >= len(callee.Params) {
			continue
		}
		parameter := callee.Params[index]
		directInvocation := func(candidate ssa.Instruction) bool {
			called := ssaflow.InstructionCall(candidate)
			return called != nil && called.Value == parameter
		}
		if !ssaflow.UnownedReturnFromEntryAssumingNonNil(callee, parameter, directInvocation) {
			return true
		}
	}
	return false
}

func (classifier *cancellationClassifier) returnAction(returned *ssa.Return) cancellationAction {
	if slices.Contains(returned.Results, classifier.cancel) {
		classifier.transfers = true
		return cancellationActionTransfer
	}
	if ssaflow.ReturnedValueOwnsValue(returned, classifier.cancel) {
		return cancellationActionUnknown
	}
	return cancellationActionNone
}

func commonHasExactArgument(common *ssa.CallCommon, target ssa.Value) bool {
	return slices.Contains(common.Args, target)
}

func instructionReferencesCancellation(instruction ssa.Instruction, cancel ssa.Value) bool {
	for _, operand := range instruction.Operands(nil) {
		if operand == nil || *operand == nil {
			continue
		}
		if *operand == cancel || ssaflow.SameValue(*operand, cancel) || ssaflow.ValueContainsValue(*operand, cancel) ||
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
		if ok && store.Addr == value && (store.Val == cancel || ssaflow.SameValue(store.Val, cancel)) {
			return true
		}
	}
	return false
}

func deferredClosureUseIsLocallyResolved(instruction ssa.Instruction, cancel ssa.Value) bool {
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
		if !newCancellationUse().parameterResolved(function, captured.Free) {
			return false
		}
	}
	return found
}

func localCallOnlyObserves(instruction ssa.Instruction, cancel ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) == 0 {
		return false
	}
	callee := common.StaticCallee()
	found := false
	for index, argument := range common.Args {
		closureContainsCancel := false
		if _, ok := argument.(*ssa.MakeClosure); ok {
			closureContainsCancel = ssaflow.ValueContainsValue(argument, cancel)
		}
		if index >= len(callee.Params) || argument != cancel && !closureContainsCancel {
			continue
		}
		found = true
		if !newCancellationUse().parameterResolved(callee, callee.Params[index]) {
			return false
		}
	}
	return found
}

// cancellationUse answers whether every use of a cancel value inside a callee
// is resolved. The memo owns the cycle guard and the rule that an answer cut
// short by it is not retained.
type cancellationUse struct {
	memo *ssaflow.CallGraphMemo[cancellationUseKey, bool]
}

type cancellationUseKey struct {
	function  *ssa.Function
	parameter ssa.Value
}

func newCancellationUse() *cancellationUse {
	return &cancellationUse{memo: ssaflow.NewCallGraphMemo[cancellationUseKey, bool]()}
}

func (search *cancellationUse) parameterResolved(function *ssa.Function, parameter ssa.Value) bool {
	return search.memo.Answer(cancellationUseKey{function: function, parameter: parameter}, func() bool {
		return search.searchParameterResolved(function, parameter)
	})
}

func (search *cancellationUse) searchParameterResolved(function *ssa.Function, parameter ssa.Value) bool {
	if !search.memo.Enter(function) {
		return false
	}
	defer search.memo.Leave(function)
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
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
	for index, argument := range common.Args {
		if index >= len(callee.Params) || !exactLocalValueUse(argument, parameter) {
			continue
		}
		matched = true
		if !search.parameterResolved(callee, callee.Params[index]) {
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
