package goroutineownership

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/ssa"
)

// The classifier labels each instruction of the spawning function with the
// effect it has on the worker's tracked values. A join is an exact observation
// of completion; a transfer hands the exact value to a caller or an owner that
// outlives the function; unknown covers any other consumption the analysis
// cannot see through, such as an opaque call, a send, or a helper that lets
// the value escape. Anything that does not touch a tracked value is none.
// Unknown is deliberately not a weaker join: it suppresses the diagnostic
// instead of proving ownership.

type ownershipAction uint8

const (
	actionNone ownershipAction = iota
	actionJoin
	actionTransfer
	actionUnknown
)

func (action ownershipAction) String() string {
	switch action {
	case actionJoin:
		return "join"
	case actionTransfer:
		return "transfer"
	case actionUnknown:
		return "opaque-use"
	case actionNone:
	}
	return "none"
}

// obligation maps this classifier's labels onto the shared flow lattice: a
// join or transfer is exact evidence and an opaque use is unknown.
func (action ownershipAction) obligation() ssaflow.ObligationAction {
	switch action {
	case actionJoin, actionTransfer:
		return ssaflow.ObligationExact
	case actionUnknown:
		return ssaflow.ObligationUnknown
	case actionNone:
	}
	return ssaflow.ObligationNone
}

func (analysis *spawnAnalysis) obligation(instruction ssa.Instruction) ssaflow.ObligationAction {
	return analysis.action(instruction).obligation()
}

// edgeObligation credits a selected receive of a tracked signal as an exact
// join on that arm alone, and a selected receive of an opaque worker's
// context as an opaque observation on that arm alone. The exit of a counted
// select drain is an exact join on that edge alone.
func (analysis *spawnAnalysis) edgeObligation(from, to *ssa.BasicBlock) ssaflow.ObligationAction {
	if analysis.selectedJoinEdge(from, to) {
		return ssaflow.ObligationExact
	}
	if drain := analysis.countedDrainAction(from, to); drain != ssaflow.ObligationNone {
		return drain
	}
	if channel, selected := ssaflow.SelectedReceiveOnEdge(from, to); selected && analysis.possibleSignal(channel) {
		analysis.recordEdge(from, to, reasonSelectedPossibleReceiveEdge, ssaflow.ObligationUnknown)
		return ssaflow.ObligationUnknown
	}
	if analysis.selectedOwnershipEdge(from, to) {
		return ssaflow.ObligationUnknown
	}
	return ssaflow.ObligationNone
}

// strongerAction merges the labels of several tracked values touched by one
// instruction. A proven join wins because it already required every-return
// coverage; otherwise any escape keeps the instruction opaque.
func strongerAction(current, next ownershipAction) ownershipAction {
	switch {
	case current == actionJoin || next == actionJoin:
		return actionJoin
	case current == actionUnknown || next == actionUnknown:
		return actionUnknown
	case current == actionTransfer || next == actionTransfer:
		return actionTransfer
	default:
		return actionNone
	}
}

func (analysis *spawnAnalysis) action(instruction ssa.Instruction) ownershipAction {
	if action, ok := analysis.actions[instruction]; ok {
		return action
	}
	action, reason := analysis.classify(instruction)
	analysis.actions[instruction] = action
	analysis.traceLabel(instruction, action, reason)
	return action
}

// traceLabel records a join, transfer, or opaque-use label once, when the
// instruction is first classified, so the trace lists labels in the order the
// walk met them, each with the rule that produced it. An instruction labelled
// none is not traced.
func (analysis *spawnAnalysis) traceLabel(instruction ssa.Instruction, action ownershipAction, reason goroutineOwnershipReason) {
	if action == actionNone || !analysis.probe.Enabled() {
		return
	}
	outcome := analysisTrace.OutcomeAccepted
	if action == actionUnknown {
		outcome = analysisTrace.OutcomeUnknown
	}
	analysis.probe.Label(analysisTrace.Step{
		Reason: reason.String(), Outcome: outcome, Pos: instruction.Pos(), Function: analysis.function.String(),
		Details: map[string]string{"instruction": instruction.String(), "label": action.String()},
	})
}

// classify labels the instruction and names the rule that decided it, so a
// trace of an opaque use says which boundary it was.
func (analysis *spawnAnalysis) classify(instruction ssa.Instruction) (ownershipAction, goroutineOwnershipReason) {
	if call, ok := instruction.(*ssa.Call); ok && storedTerminationReceiver(call.Common()) {
		return actionUnknown, reasonLabelStoredTestingReceiver
	}
	if selectedReceiveAtEntry(instruction, analysis.isSignal) {
		return actionJoin, reasonLabelSignalReceived
	}
	if selectedReceiveAtEntry(instruction, analysis.possibleSignal) {
		return actionUnknown, reasonLabelPossibleSignalReceive
	}
	switch typed := instruction.(type) {
	case *ssa.Return:
		// Return ownership uses the same cache as other instructions: a merged
		// return or a guarded re-walk must not repeat the query or its label.
		return analysis.returnAction(typed)
	case *ssa.MakeClosure:
		// Capturing a value has no effect by itself. The closure's defer,
		// return, store, launch, or opaque call is classified where it happens.
		return actionNone, reasonNone
	case *ssa.UnOp, *ssa.Select, *ssa.Range:
		if guaranteedReceive(instruction, analysis.isSignal) {
			return actionJoin, reasonLabelSignalReceived
		}
		if guaranteedReceive(instruction, analysis.possibleSignal) {
			return actionUnknown, reasonLabelPossibleSignalReceive
		}
		if receivesFrom(instruction, analysis.signalAggregateCarries) {
			return actionUnknown, reasonLabelSignalAggregateReceive
		}
		if analysis.selectSends(instruction) {
			return actionUnknown, reasonLabelSelectSends
		}
	case *ssa.Store:
		return analysis.storeAction(typed), reasonLabelStoredOutside
	case *ssa.Send:
		if analysis.consumes(typed.X) {
			return actionUnknown, reasonLabelSent
		}
	case *ssa.MapUpdate:
		if analysis.consumes(typed.Value) {
			return actionUnknown, reasonLabelStoredInMap
		}
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return analysis.callAction(instruction, ssaflow.InstructionCall(instruction))
	}
	return actionNone, reasonNone
}

// Capturing a strict testing object introduces a receiver load that the
// storage-free contract registry deliberately cannot resolve. Ask the existing
// storage proof for its immutable origin, then delegate policy to that registry.
// This is a terminal-return boundary, never an assertion that the worker joined.
// https://github.com/ConduitIO/conduit/blob/9946a19b9fff997675f78bbc5ff437e760d39f4f/pkg/lifecycle/stream/destination_acker_test.go#L30-L92
func storedTerminationReceiver(common *ssa.CallCommon) bool {
	receiver := ssaflow.CallReceiver(common)
	if receiver == nil || len(common.Args) == 0 || common.Args[0] != receiver {
		return false
	}
	resolved := heapmodel.NewStorage(nil).Resolve(receiver)
	if !resolved.Proven() || resolved.Value == receiver {
		return false
	}
	copy := *common
	copy.Args = slices.Clone(common.Args)
	copy.Args[0] = resolved.Value
	return ssaflow.HasLibraryContract(&copy, ssaflow.ContractTestingTermination)
}

// An exact returned handle covers the obligation even if another result has
// opaque containment. A possible handle alone never establishes ownership.
func (analysis *spawnAnalysis) returnAction(returned *ssa.Return) (ownershipAction, goroutineOwnershipReason) {
	result := actionNone
	for _, value := range returned.Results {
		action := analysis.transferAction(value)
		if action == actionTransfer {
			return actionTransfer, reasonLabelReturnedTracked
		}
		result = strongerAction(result, action)
	}
	if result == actionUnknown {
		return actionUnknown, reasonLabelReturnedContainment
	}
	if analysis.returnMayTransfer(returned) {
		return actionUnknown, reasonLabelReturnedProjection
	}
	return actionNone, reasonNone
}

// A signal mapped only to its captured aggregate may leave through a field
// projection. Matching the root establishes possible handoff, not the exact
// field or a guaranteed join, so only the unknown-return query uses it.
// https://github.com/mysteriumnetwork/node/blob/c45527af1ea80300ae3d9c92bd37255335b6140d/session/pingpong/hermes_promise_handler.go#L120-L140
func (analysis *spawnAnalysis) returnMayTransfer(returned *ssa.Return) bool {
	return slices.ContainsFunc(returned.Results, func(result ssa.Value) bool {
		root := aggregateRoot(result)
		return slices.ContainsFunc(analysis.signals, func(signal ssa.Value) bool {
			return !ssaflow.ChannelType(signal) && heapmodel.MayAlias(root, aggregateRoot(signal))
		})
	})
}

// A store outside the function transfers only an exact tracked value. Possible
// containment is opaque ownership; local storage changes nothing until handoff.
func (analysis *spawnAnalysis) storeAction(store *ssa.Store) ownershipAction {
	switch address := store.Addr.(type) {
	case *ssa.Global, *ssa.FreeVar:
		return analysis.transferAction(store.Val)
	case *ssa.FieldAddr:
		if ssaflow.ExternallyOwnedValue(address.X) {
			return analysis.transferAction(store.Val)
		}
	}
	return actionNone
}

func (analysis *spawnAnalysis) callAction(instruction ssa.Instruction, common *ssa.CallCommon) (ownershipAction, goroutineOwnershipReason) {
	if common == nil {
		return actionNone, reasonNone
	}
	// A launched observer runs independently of the caller. Handle it before
	// receiver, summary and library contracts that describe synchronous calls.
	if _, launched := instruction.(*ssa.Go); launched {
		_, closure := ssaflow.DirectCallee(common)
		return analysis.opaqueCallAction(common, closure, reasonLabelLaunchedHelper)
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		// append retains its arguments in a slice the caller keeps; the other
		// builtins only observe a channel or its capacity.
		if builtin.Name() == "append" && analysis.anyArgumentConsumes(common) {
			return actionUnknown, reasonLabelAppended
		}
		return actionNone, reasonNone
	}
	if action, reason := analysis.directJoinAction(common); action != actionNone {
		return action, reason
	}
	owner := analysis.closesRetainedWorkerOwner(instruction, common)
	if owner.Reason == ssaflow.EvidenceBudgetExhausted {
		return actionUnknown, reasonRetainedOwnerBudgetExhausted
	}
	if owner.Proven() {
		return actionUnknown, reasonLabelClosesRetainedOwner
	}
	if action := analysis.pipePeerAction(instruction, common); action != actionNone {
		return action, reasonLabelPipePeer
	}
	if analysis.summarizedJoin(instruction) {
		return actionJoin, reasonLabelSummaryJoin
	}
	if analysis.waitGroupBookkeeping(common) {
		return actionNone, reasonNone
	}
	if ssaflow.HasLibraryContract(common, ssaflow.ContractTestingCleanup) {
		return analysis.testingCleanupAction(common), reasonLabelTestingCleanup
	}
	if ssaflow.HasLibraryContract(common, ssaflow.ContractGoMockReturn) && analysis.anyArgumentConsumes(common) {
		// gomock.Return publishes its configured results, but broad argument
		// containment does not prove the exact stream is among those results.
		// https://github.com/uber-go/mock/blob/539d81c0f42174d17e8f91abcb869bed37605a15/gomock/call.go#L185-L205
		return actionUnknown, reasonLabelGoMockReturn
	}
	callee, closure := ssaflow.DirectCallee(common)
	if callee == nil || len(callee.Blocks) == 0 {
		// An opaque callee may retain the value.
		if callee == nil {
			return analysis.opaqueCallAction(common, closure, reasonLabelDynamicCallee)
		}
		return analysis.opaqueCallAction(common, closure, reasonLabelCalleeWithoutBody)
	}
	return analysis.helperAction(common, callee, closure, analysis.tracked), reasonLabelHelper
}

// Opaque and launched calls share the same handoff boundary. Only positive
// argument or capture consumption makes the tracked worker uncertain.
func (analysis *spawnAnalysis) opaqueCallAction(
	common *ssa.CallCommon, closure *ssa.MakeClosure, reason goroutineOwnershipReason,
) (ownershipAction, goroutineOwnershipReason) {
	if analysis.anyArgumentConsumes(common) || analysis.closureConsumes(closure) {
		return actionUnknown, reason
	}
	return actionNone, reasonNone
}

// Wait observes completion only on the exact settling group. A lifecycle
// method on a captured owner supplies possible shutdown participation; neither
// its name nor receiver identity proves observation of this worker's completion.
func (analysis *spawnAnalysis) directJoinAction(common *ssa.CallCommon) (ownershipAction, goroutineOwnershipReason) {
	receiver := ssaflow.CallReceiver(common)
	if receiver == nil {
		return actionNone, reasonNone
	}
	if !ssaflow.CallMatchesSymbol(common, waitGroupWait) {
		if lifecycleMethod(ssaflow.CallName(common)) && ownerReceiver(receiver, analysis.owners) {
			return actionUnknown, reasonLabelOwnerLifecycle
		}
		return actionNone, reasonNone
	}
	storage := heapmodel.NewStorage(analysis.budget())
	for _, target := range analysis.groups {
		if storage.Same(receiver, target).Proven() {
			return actionJoin, reasonLabelDirectJoin
		}
	}
	if heapmodel.MayAliasAny(receiver, analysis.groups) || storage.Budget().Exhausted() {
		return actionUnknown, reasonLabelPossibleJoin
	}
	return actionNone, reasonNone
}

var waitGroupAdd = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Add"})

var waitGroupGo = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Go"})

// waitGroupBookkeeping recognizes the documented sync.WaitGroup counter
// methods on a tracked group. They neither observe completion nor let the
// group escape, so they must not make the group opaque.
func (analysis *spawnAnalysis) waitGroupBookkeeping(common *ssa.CallCommon) bool {
	receiver := ssaflow.CallReceiver(common)
	if receiver == nil || !heapmodel.MayAliasAny(receiver, analysis.groups) && !analysis.unsettledGroup(receiver) {
		return false
	}
	return waitGroupMethod(common)
}

func waitGroupMethod(common *ssa.CallCommon) bool {
	return ssaflow.CallMatchesSymbol(common, waitGroupAdd) || ssaflow.CallMatchesSymbol(common, waitGroupDone) ||
		ssaflow.CallMatchesSymbol(common, waitGroupGo) || ssaflow.CallMatchesSymbol(common, waitGroupWait)
}

// unsettledGroup keeps an early-Done WaitGroup out of the opaque set so the
// readiness bookkeeping cannot hide an independent completion obligation.
func (analysis *spawnAnalysis) unsettledGroup(receiver ssa.Value) bool {
	if analysis.unsettledDone == nil {
		return false
	}
	pointer, ok := receiver.Type().Underlying().(*types.Pointer)
	return ok && syntax.NamedType(pointer.Elem(), "sync", "WaitGroup")
}

// helperAction follows every tracked value that the call site supplies to a
// source-visible callee, whether as an argument or a captured variable.
func (analysis *spawnAnalysis) helperAction(
	common *ssa.CallCommon, callee *ssa.Function, closure *ssa.MakeClosure, values []trackedValue,
) ownershipAction {
	result := actionNone
	for _, pair := range ssaflow.CallBindings(common, callee, closure) {
		for _, tracked := range values {
			carried := bindingCarries(pair.Supplied, tracked.value)
			projected := ssaflow.ValueIsAccessPathFrom(tracked.value, pair.Supplied)
			if !carried && !projected {
				continue
			}
			search := newHelperSearch()
			search.concurrency, _ = summaryKnowledge.Provider(analysis.pass).Concurrency()
			action := analysis.boundHelperAction(pair.Supplied, tracked, search.use(callee, pair.Local, tracked.kind))
			result = strongerAction(result, action)
		}
	}
	return result
}

// Owner coverage establishes a lifecycle call, not observation of worker
// completion. A completion handle also needs exact binding: containment includes
// old stores and aggregate projections, which supply only possible shutdown.
// https://github.com/jech/galene/blob/6d9338e909fdecdd906150e4dda34e10d9869654/rtpconn/webclient.go#L878-L894
func (analysis *spawnAnalysis) boundHelperAction(supplied ssa.Value, tracked trackedValue, effect ownershipAction) ownershipAction {
	if effect != actionJoin {
		return effect
	}
	if tracked.kind == trackedOwner {
		return actionUnknown
	}
	if !heapmodel.NewStorage(analysis.budget()).Same(supplied, tracked.value).Proven() {
		return actionUnknown
	}
	return actionJoin
}

// testingCleanupAction treats a testing Cleanup callback like a deferred
// helper: testing guarantees that it runs after the test completes, so a join
// on every path through the callback settles the worker.
// https://github.com/charmbracelet/crush/blob/6fa9e6905041c32ffceb1c9b1a3189b3db1eec07/internal/server/socket_test.go#L162-L177
func (analysis *spawnAnalysis) testingCleanupAction(common *ssa.CallCommon) ownershipAction {
	result := actionNone
	for _, argument := range common.Args {
		closure, ok := argument.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		if callee, _ := closure.Fn.(*ssa.Function); callee != nil {
			result = strongerAction(result, analysis.helperAction(nil, callee, closure, analysis.tracked))
		}
	}
	return result
}

// receivesFrom reports whether instruction receives from a channel accepted by
// matches, through a receive expression, a select case, or a channel range.
func receivesFrom(instruction ssa.Instruction, matches func(ssa.Value) bool) bool {
	switch typed := instruction.(type) {
	case *ssa.UnOp:
		return typed.Op == token.ARROW && matches(typed.X)
	case *ssa.Select:
		for _, state := range typed.States {
			if state.Dir == types.RecvOnly && matches(state.Chan) {
				return true
			}
		}
	case *ssa.Range:
		_, channel := typed.X.Type().Underlying().(*types.Chan)
		return channel && matches(typed.X)
	}
	return false
}

// A mixed select does not settle every path. Its exact receive arm is credited
// on the edge, or at an unambiguous arm entry, keeping other cases open.
// https://github.com/containerd/stargz-snapshotter/blob/624678b4e421947534cbf0618f9609853cccee0f/store/manager.go#L193-L221
func guaranteedReceive(instruction ssa.Instruction, matches func(ssa.Value) bool) bool {
	if selectedReceiveAtEntry(instruction, matches) {
		return true
	}
	if choice, ok := instruction.(*ssa.Select); ok {
		return choice.Blocking && len(choice.States) > 0 && !slices.ContainsFunc(choice.States, func(state *ssa.SelectState) bool {
			return state.Dir != types.RecvOnly || !matches(state.Chan)
		})
	}
	return receivesFrom(instruction, matches)
}

func selectedReceiveAtEntry(instruction ssa.Instruction, matches func(ssa.Value) bool) bool {
	block := instruction.Block()
	if len(block.Instrs) == 0 || block.Instrs[0] != instruction {
		return false
	}
	channel, selected := ssaflow.SelectedReceiveChannel(block)
	return selected && matches(channel)
}

func (analysis *spawnAnalysis) selectedJoinEdge(from, to *ssa.BasicBlock) bool {
	channel, selected := ssaflow.SelectedReceiveOnEdge(from, to)
	joined := selected && analysis.isSignal(channel)
	if joined {
		analysis.recordEdge(from, to, reasonSelectedReceiveEdge, ssaflow.ObligationExact)
	}
	return joined
}

// recordEdge keeps the authoritative reason and action for the trace. Tracing
// projects the action instead of reconstructing proof strength from reason names.
func (analysis *spawnAnalysis) recordEdge(from, to *ssa.BasicBlock, reason goroutineOwnershipReason, action ssaflow.ObligationAction) {
	if !analysis.tracing {
		return
	}
	if analysis.edgeEvidence == nil {
		analysis.edgeEvidence = make(map[[2]int]joinEdgeEvidence)
	}
	analysis.edgeEvidence[[2]int{from.Index, to.Index}] = joinEdgeEvidence{reason: reason, action: action}
}

// selectSends reports whether a select statement offers a tracked value on a
// send case; like a plain send, the receiver may retain it.
func (analysis *spawnAnalysis) selectSends(instruction ssa.Instruction) bool {
	choice, ok := instruction.(*ssa.Select)
	if !ok {
		return false
	}
	return slices.ContainsFunc(choice.States, func(state *ssa.SelectState) bool {
		return state.Dir == types.SendOnly && analysis.consumes(state.Send)
	})
}

// isSignal requires exact channel identity. An aggregate root or a phi with
// one matching alternative explains possible completion, never an exact join.
func (analysis *spawnAnalysis) isSignal(value ssa.Value) bool {
	return analysis.isSignalWithin(value, analysis.budget())
}

// isSignalWithin shares exact channel identity between observations and relay
// discovery. Aggregate roots retain possible ownership, never an exact join.
func (analysis *spawnAnalysis) isSignalWithin(value ssa.Value, budget *ssaflow.SearchBudget) bool {
	if !ssaflow.ChannelType(value) {
		return false
	}
	storage := heapmodel.NewStorage(budget)
	for _, signal := range analysis.signals {
		if !budget.Spend() {
			return false
		}
		if ssaflow.ChannelType(signal) && storage.Same(value, signal).Proven() {
			return true
		}
	}
	return false
}

// possibleSignal retains the previous broad receive boundary for unknown
// ownership and guarded joins. Matching sibling fields/indexes does not show
// that the caller observed this worker's completion handle.
func (analysis *spawnAnalysis) possibleSignal(value ssa.Value) bool {
	if heapmodel.MayAliasAny(value, analysis.signals) {
		return true
	}
	root := aggregateRoot(value)
	return root != value && slices.ContainsFunc(analysis.signals, func(signal ssa.Value) bool {
		if !ssaflow.ChannelType(signal) {
			// Captured slice cells and their loaded slice share an aggregate root.
			// Index correlation stays unknown under countedJoin, not proven exact.
			// https://github.com/bazel-contrib/buildtools/blob/933e9bbe17f7619afaca1dd58ce22810042f1c13/buildifier/buildifier.go#L241-L268
			return heapmodel.MayAlias(ssaflow.CapturedBindingValue(root), ssaflow.CapturedBindingValue(aggregateRoot(signal)))
		}
		signalRoot := aggregateRoot(signal)
		return signalRoot != signal && heapmodel.MayAlias(root, signalRoot)
	})
}

// A worker-side field can resolve only to the constructor's aggregate result,
// while its caller receives the original channel passed to that constructor.
// Containment cannot identify the exact field, so this is unknown, not a join.
// https://github.com/lotusirous/go-concurrency-patterns/blob/791337ff11e69cd9d1a8587e53474a8469b3510f/17-ring-buffer-channel/main.go#L49-L62
func (analysis *spawnAnalysis) signalAggregateCarries(value ssa.Value) bool {
	return slices.ContainsFunc(analysis.signals, func(signal ssa.Value) bool {
		return !ssaflow.ChannelType(signal) && carries(ssaflow.NewReachingWalk(carryForms), signal, value)
	})
}

func (analysis *spawnAnalysis) anyArgumentConsumes(common *ssa.CallCommon) bool {
	return slices.ContainsFunc(common.Args, analysis.consumes)
}

func (analysis *spawnAnalysis) closureConsumes(closure *ssa.MakeClosure) bool {
	if closure == nil {
		return false
	}
	return slices.ContainsFunc(closure.Bindings, func(binding ssa.Value) bool {
		return slices.ContainsFunc(analysis.tracked, func(tracked trackedValue) bool {
			return bindingCarries(binding, tracked.value)
		})
	})
}
