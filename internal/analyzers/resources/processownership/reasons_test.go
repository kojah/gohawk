package processownership

import "testing"

func TestProcessReasonCodes(t *testing.T) {
	want := map[processReason]string{
		reasonNone:                   "",
		reasonHelperOwnershipUnknown: "helper-command-ownership-unknown",
		reasonStartFailureReturn:     "start-failure-return",
		reasonImpossibleNilReturn:    "impossible-nil-process-return",
		reasonReturnedOwner:          "returned-value-owns-command",
		reasonReturnedHandle:         "returns-process-handle",
		reasonReturnedMergedOwner:    "returned-value-owns-merged-command",
		reasonWaitOwnershipProven:    "wait-ownership-proven",
		reasonUnownedReturn:          "unowned-return",
		reasonAmbiguousWaitOwnership: "ambiguous-wait-ownership",
	}
	if len(want) != int(processReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range processReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []processReason{processReasonCount, 255} {
		if reason.String() != "invalid-process-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
