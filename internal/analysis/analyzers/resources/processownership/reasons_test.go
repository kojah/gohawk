package processownership

import "testing"

func TestProcessReasonCodes(t *testing.T) {
	want := map[processReason]string{
		reasonCallerCommandOwnershipUnknown:    "caller-command-ownership-unknown",
		reasonAggregateCommandOwnershipUnknown: "aggregate-command-ownership-unknown",
		reasonPreStartOwnershipUnknown:         "pre-start-ownership-unknown",
		reasonSuccessfulStartCannotReturn:      "successful-start-cannot-return",
		reasonLocalWaitObligation:              "local-wait-obligation",
		reasonPreStartCutoff:                   "budget-exhausted",
		reasonPreStartEvidenceUnavailable:      "evidence-unavailable",
		reasonCommandUseCutoff:                 "command-use-budget-exhausted",
		reasonNone:                             "",
		reasonHelperOwnershipUnknown:           "helper-command-ownership-unknown",
		reasonStartFailureReturn:               "start-failure-return",
		reasonSuccessfulStartProcessNonNil:     "successful-start-process-non-nil",
		reasonReturnedOwner:                    "returned-value-owns-command",
		reasonReturnedHandle:                   "returns-process-handle",
		reasonReturnedMergedOwner:              "returned-value-owns-merged-command",
		reasonWaitOwnershipProven:              "wait-ownership-proven",
		reasonUnownedReturn:                    "unowned-return",
		reasonUnusedCommandOwnershipUnknown:    "unused-command-ownership-unknown",
		reasonAmbiguousWaitOwnership:           "ambiguous-wait-ownership",
		reasonProgramLifetimeOwnershipUnknown:  "program-lifetime-ownership-unknown",
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
