package goroutineownership

import "testing"

func TestGoroutineOwnershipReasonCodes(t *testing.T) {
	want := map[goroutineOwnershipReason]string{
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
	}
	if len(want) != int(goroutineOwnershipReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range goroutineOwnershipReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []goroutineOwnershipReason{goroutineOwnershipReasonCount, 255} {
		if reason.String() != "invalid-goroutine-ownership-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
