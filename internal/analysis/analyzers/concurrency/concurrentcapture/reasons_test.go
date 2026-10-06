package concurrentcapture

import "testing"

func TestCaptureReasonCodes(t *testing.T) {
	want := map[captureReason]string{
		reasonNone:                 "",
		reasonRepeatedWrite:        "capture-repeated-write",
		reasonLockFallbackUnknown:  "capture-lock-fallback-unknown",
		reasonWorkerGuardUnknown:   "capture-worker-guard-unknown",
		reasonChannelGuardUnknown:  "capture-channel-guard-unknown",
		reasonUnguardedWrite:       "capture-unguarded-write",
		reasonWorkerOrderUnknown:   "capture-worker-order-unknown",
		reasonMutationSiteUnknown:  "capture-mutation-site-unknown",
		reasonHelperEffectsUnknown: "capture-helper-effects-unknown",
		reasonLockIdentityUnknown:  "capture-lock-identity-unknown",
		reasonLockHeld:             "capture-lock-held",
		reasonNoLockHeld:           "capture-no-lock-held",
	}
	if len(want) != int(captureReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range captureReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []captureReason{captureReasonCount, 255} {
		if reason.String() != "invalid-capture-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
