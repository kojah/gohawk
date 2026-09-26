package cancellationownership

import "testing"

func TestCancellationReasonCodes(t *testing.T) {
	want := map[cancellationReason]string{
		reasonCancellationNone: "", reasonCancellationUnknown: "ambiguous-cancellation-use",
		reasonCancellationReleased: "exact-cancellation-release", reasonCancellationTransferred: "exact-cancellation-transfer",
		reasonCancellationLost: "unowned-return", reasonLabelRelease: "release",
		reasonLabelOpaqueUse: "opaque-cancellation-use", reasonLabelParentContextUse: "parent-context-use",
		reasonLabelLaunchedCancel:         "launched-cancel",
		reasonLabelOwnDoneReceive:         "own-done-receive",
		reasonLabelDeferredClosure:        "deferred-closure-may-cancel",
		reasonLabelReturnedCallback:       "returned-callback-release",
		reasonLabelTestingCleanup:         "testing-cleanup",
		reasonLabelRegisteredCallback:     "registered-callback",
		reasonLabelLaunchedHelper:         "launched-helper",
		reasonLabelHelperRelease:          "helper-release",
		reasonLabelSummaryRelease:         "summary-release",
		reasonLabelHelperUndecided:        "helper-completion-unknown",
		reasonLabelHelperMayInvoke:        "helper-may-invoke",
		reasonLabelReturnsDeferredCleanup: "returned-deferred-cleanup",
		reasonLabelCapturedByCallback:     "captured-by-callback",
		reasonLabelStored:                 "stored",
		reasonLabelSent:                   "sent",
		reasonLabelStoredInMap:            "stored-in-map",
		reasonLabelReturned:               "returned", reasonLabelAliased: "aliased",
		reasonLabelPassedToCallee: "passed-to-callee",
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
