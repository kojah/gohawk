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
	reasonSharedStorageSignal
	reasonNoObligation
	reasonUnownedReturn
	reasonDoneBeforeCompletion
	reasonSelectedReceiveEdge
	reasonSelectedContextEdge
	reasonCountedDrainEdge
	reasonProcessExitStopsWorker
	goroutineOwnershipReasonCount
)

var ownershipReasonCodes = [...]string{
	reasonNone:                    "",
	reasonJoinProven:              "join-proven",
	reasonDeferredJoinBeforeSpawn: "deferred-join-before-spawn",
	reasonGuardedLocalJoin:        "guarded-local-join",
	reasonStopLifecycle:           "stop-lifecycle",
	reasonContextLifecycle:        "context-lifecycle",
	reasonLocallyCanceledContext:  "locally-canceled-context",
	reasonReceiverContext:         "receiver-context-lifecycle",
	reasonRelayDependency:         "relay-dependency-lifecycle",
	reasonSynctestBubbleOwner:     "synctest-bubble-owner",
	reasonCallerOrExternalOwner:   "caller-or-external-owner",
	reasonOwnershipTransfer:       "ownership-transfer",
	reasonOpaqueTransfer:          "opaque-ownership-transfer",
	reasonLoopJoinUnproven:        "loop-join-unproven",
	reasonWorkerConsumesSignal:    "signal-consumed-by-worker",
	reasonFlagGuardedJoin:         "flag-guarded-join",
	reasonBufferedSignal:          "buffered-completion-signal",
	reasonSharedStorageSignal:     "shared-storage-signal",
	reasonNoObligation:            "no-completion-obligation",
	reasonUnownedReturn:           "unowned-return",
	reasonDoneBeforeCompletion:    "waitgroup-done-before-completion",
	reasonSelectedReceiveEdge:     "selected-receive-edge",
	reasonSelectedContextEdge:     "selected-context-edge",
	reasonCountedDrainEdge:        "counted-drain-edge",
	reasonProcessExitStopsWorker:  "process-exit-stops-worker",
}

func (reason goroutineOwnershipReason) String() string {
	if int(reason) >= len(ownershipReasonCodes) {
		return "invalid-goroutine-ownership-reason"
	}
	return ownershipReasonCodes[reason]
}
