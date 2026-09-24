package nilargument

import "testing"

func TestNilArgumentReasonCodes(t *testing.T) {
	want := map[nilArgumentReason]string{
		reasonNone:                    "",
		reasonCalleeDereferences:      "callee-dereferences-argument",
		reasonSlotTypeUnknown:         "slot-type-unknown",
		reasonSlotNotPointer:          "slot-not-pointer",
		reasonSlotNotProvenNil:        "slot-not-proven-nil",
		reasonNilSlotDereferenced:     "nil-slot-dereferenced",
		reasonEarlierCallUnsummarized: "earlier-call-unsummarized",
		reasonEarlierCalls:            "earlier-calls",
	}
	if len(want) != int(nilArgumentReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range nilArgumentReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []nilArgumentReason{nilArgumentReasonCount, 255} {
		if reason.String() != "invalid-nil-argument-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
