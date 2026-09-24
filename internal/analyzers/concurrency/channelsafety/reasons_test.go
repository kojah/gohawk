package channelsafety

import "testing"

func TestSafetyReasonCodes(t *testing.T) {
	want := map[safetyReason]string{
		safetyReasonNone:             "",
		safetyReasonReachableSend:    "send-reachable-after-close",
		safetyReasonIdentityUnproven: "send-channel-identity-not-proven",
		safetyReasonSendAfterClose:   "send-after-close-proven",
	}
	if len(want) != int(safetyReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range safetyReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []safetyReason{safetyReasonCount, 255} {
		if reason.String() != "invalid-channel-safety-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
