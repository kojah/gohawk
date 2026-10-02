package goroutineownership

// goroutineOwnershipReason describes policy evidence without encoding it as text.
type goroutineOwnershipReason uint8

const (
	reasonNone goroutineOwnershipReason = iota
	reasonJoinProven
	reasonDeferredJoinBeforeSpawn
	reasonGuardedLocalJoin
	reasonStopLifecycle
	reasonContextLifecycle
	reasonLocallyCanceledContext
	reasonReceiverContext
	reasonRelayDependency
	reasonSynctestBubbleOwner
	reasonCallerOrExternalOwner
	reasonOwnershipTransfer
	reasonOpaqueTransfer
	reasonLoopJoinUnproven
	reasonWorkerConsumesSignal
	reasonFlagGuardedJoin
	reasonBufferedSignal
	reasonUnobservedSignal
	reasonSignalCensusUnavailable
	reasonSharedStorageSignal
	reasonNoObligation
	reasonUnownedReturn
	reasonDoneBeforeCompletion
	reasonSelectedReceiveEdge
	reasonSelectedContextEdge
	reasonCountedDrainEdge
	reasonProcessExitStopsWorker
	reasonPreSpawnCensusCutoff
	reasonReceiveBudgetExhausted
	reasonDiscoveryBudgetExhausted
	reasonRetainedOwnerBudgetExhausted
	reasonRelayDependencyBudgetExhausted
	// The label reasons say why the classifier labelled one instruction.
	reasonLabelSignalReceived
	reasonLabelDirectJoin
	reasonLabelSummaryJoin
	reasonLabelHelper
	reasonLabelPipePeer
	reasonLabelTestingCleanup
	reasonLabelStoredOutside
	reasonLabelGoMockReturn
	reasonLabelStoredTestingReceiver
	reasonLabelSignalAggregateReceive
	reasonLabelSelectSends
	reasonLabelSent
	reasonLabelStoredInMap
	reasonLabelAppended
	reasonLabelClosesRetainedOwner
	reasonLabelDynamicCallee
	reasonLabelCalleeWithoutBody
	reasonLabelLaunchedHelper
	reasonLabelReturnedTracked
	reasonLabelReturnedProjection
	reasonLabelReturnedContainment
	reasonLabelPossibleJoin
	reasonLabelOwnerLifecycle
	reasonLabelPossibleSignalReceive
	reasonSelectedPossibleReceiveEdge
	reasonCountedPossibleDrainEdge
	goroutineOwnershipReasonCount
)

var ownershipReasonCodes = [...]string{
	reasonPreSpawnCensusCutoff:           "pre-spawn-census-budget-exhausted",
	reasonRelayDependencyBudgetExhausted: "relay-dependency-budget-exhausted",
	reasonRetainedOwnerBudgetExhausted:   "retained-owner-budget-exhausted",
	reasonDiscoveryBudgetExhausted:       "completion-discovery-budget-exhausted",
	reasonReceiveBudgetExhausted:         "worker-receive-budget-exhausted",
	reasonNone:                           "",
	reasonJoinProven:                     "join-proven",
	reasonDeferredJoinBeforeSpawn:        "deferred-join-before-spawn",
	reasonGuardedLocalJoin:               "guarded-local-join",
	reasonStopLifecycle:                  "stop-lifecycle",
	reasonContextLifecycle:               "context-lifecycle",
	reasonLocallyCanceledContext:         "locally-canceled-context",
	reasonReceiverContext:                "receiver-context-lifecycle",
	reasonRelayDependency:                "relay-dependency-lifecycle",
	reasonSynctestBubbleOwner:            "synctest-bubble-owner",
	reasonCallerOrExternalOwner:          "caller-or-external-owner",
	reasonOwnershipTransfer:              "ownership-transfer",
	reasonOpaqueTransfer:                 "opaque-ownership-transfer",
	reasonLoopJoinUnproven:               "loop-join-unproven",
	reasonWorkerConsumesSignal:           "signal-consumed-by-worker",
	reasonFlagGuardedJoin:                "flag-guarded-join",
	reasonBufferedSignal:                 "buffered-completion-signal",
	reasonSignalCensusUnavailable:        "signal-census-unavailable",
	reasonUnobservedSignal:               "signal-never-observed",
	reasonSharedStorageSignal:            "shared-storage-signal",
	reasonNoObligation:                   "no-completion-obligation",
	reasonUnownedReturn:                  "unowned-return",
	reasonDoneBeforeCompletion:           "waitgroup-done-before-completion",
	reasonSelectedReceiveEdge:            "selected-receive-edge",
	reasonSelectedContextEdge:            "selected-context-edge",
	reasonCountedDrainEdge:               "counted-drain-edge",
	reasonProcessExitStopsWorker:         "process-exit-stops-worker",
	reasonLabelSignalReceived:            "signal-received",
	reasonLabelDirectJoin:                "direct-join",
	reasonLabelSummaryJoin:               "summary-join",
	reasonLabelHelper:                    "helper-use",
	reasonLabelPipePeer:                  "pipe-peer",
	reasonLabelTestingCleanup:            "testing-cleanup",
	reasonLabelStoredOutside:             "stored-outside-function",
	reasonLabelGoMockReturn:              "gomock-return",
	reasonLabelStoredTestingReceiver:     "stored-testing-receiver",
	reasonLabelSignalAggregateReceive:    "receive-from-signal-aggregate",
	reasonLabelSelectSends:               "select-sends-tracked",
	reasonLabelSent:                      "sent-to-channel",
	reasonLabelStoredInMap:               "stored-in-map",
	reasonLabelAppended:                  "appended",
	reasonLabelClosesRetainedOwner:       "closes-retained-owner",
	reasonLabelDynamicCallee:             "dynamic-callee",
	reasonLabelCalleeWithoutBody:         "callee-without-body",
	reasonLabelLaunchedHelper:            "launched-helper",
	reasonLabelReturnedTracked:           "returned-tracked-value",
	reasonLabelReturnedProjection:        "returned-signal-projection",
	reasonLabelReturnedContainment:       "returned-possible-owner",
	reasonLabelPossibleJoin:              "possible-join-receiver",
	reasonLabelOwnerLifecycle:            "owner-lifecycle-participation",
	reasonLabelPossibleSignalReceive:     "possible-signal-receive",
	reasonSelectedPossibleReceiveEdge:    "selected-possible-receive-edge",
	reasonCountedPossibleDrainEdge:       "counted-possible-drain-edge",
}

func (reason goroutineOwnershipReason) String() string {
	if int(reason) >= len(ownershipReasonCodes) {
		return "invalid-goroutine-ownership-reason"
	}
	return ownershipReasonCodes[reason]
}
