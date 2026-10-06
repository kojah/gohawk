package goroutineownership

import (
	"slices"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
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
func (action ownershipAction) obligation() ssapath.ObligationAction {
	switch action {
	case actionJoin, actionTransfer:
		return ssapath.ObligationExact
	case actionUnknown:
		return ssapath.ObligationUnknown
	case actionNone:
	}
	return ssapath.ObligationNone
}

func (analysis *spawnAnalysis) obligation(instruction ssa.Instruction) ssapath.ObligationAction {
	return analysis.action(instruction).obligation()
}

// edgeObligation credits a selected receive of a tracked signal as an exact
// join on that arm alone, and a selected receive of an opaque worker's
// context as an opaque observation on that arm alone. The exit of a counted
// select drain is an exact join on that edge alone.
func (analysis *spawnAnalysis) edgeObligation(from, to *ssa.BasicBlock) ssapath.ObligationAction {
	if analysis.selectedJoinEdge(from, to) {
		return ssapath.ObligationExact
	}
	if drain := analysis.countedDrainAction(from, to); drain != ssapath.ObligationNone {
		return drain
	}
	if channel, selected := ssapath.SelectedReceiveOnEdge(from, to); selected && analysis.possibleSignal(channel) {
		analysis.recordEdge(from, to, reasonSelectedPossibleReceiveEdge, ssapath.ObligationUnknown)
		return ssapath.ObligationUnknown
	}
	if analysis.selectedOwnershipEdge(from, to) {
		return ssapath.ObligationUnknown
	}
	return ssapath.ObligationNone
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
	return ssacall.HasLibraryContract(&copy, ssacall.ContractTestingTermination)
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
