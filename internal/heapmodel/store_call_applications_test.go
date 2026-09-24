package heapmodel

import (
	"testing"
)

func TestCallApplicationReasonCodes(t *testing.T) {
	want := map[CallApplicationReason]string{
		CallApplicationUnknown: "unknown", CallSummaryApplied: "summary-applied",
		CallNoSummary: "no-summary", CallClosure: "closure-callee", CallInterface: "interface-call",
		CallDynamic: "dynamic-call", CallStarted: "started", CallRecursive: "call-cycle",
		CallDeferredUncertain: "defer-registration-uncertain",
	}
	if len(want) != int(callApplicationReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range callApplicationReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []CallApplicationReason{callApplicationReasonCount, 255} {
		if reason.String() != "invalid-call-application-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
