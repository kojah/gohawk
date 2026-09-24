package cancellationownership

import "testing"

func TestCancellationReasonCodes(t *testing.T) {
	want := map[cancellationReason]string{
		reasonCancellationNone: "", reasonCancellationUnknown: "ambiguous-cancellation-use",
		reasonCancellationReleased: "exact-cancellation-release", reasonCancellationTransferred: "exact-cancellation-transfer",
		reasonCancellationLost: "unowned-return",
	}
	if len(want) != int(cancellationReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range cancellationReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []cancellationReason{cancellationReasonCount, 255} {
		if reason.String() != "invalid-cancellation-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
