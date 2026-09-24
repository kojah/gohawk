package deferinloop

import "testing"

func TestDeferReasonCodes(t *testing.T) {
	want := map[deferReason]string{
		reasonNone:                     "",
		reasonDeferredCleanup:          "deferred-cleanup-in-loop",
		reasonRetainedBeforeDefer:      "retained-before-defer",
		reasonLiveAtBackedge:           "live-at-backedge",
		reasonIteratorExhausted:        "iterator-exhausted",
		reasonSettledOrUnknown:         "settled-or-unknown-before-backedge",
		reasonArgumentCarriesResource:  "argument-carries-resource",
		reasonResourceTransferred:      "resource-transferred",
		reasonResourceCapturedOrStored: "resource-captured-or-stored",
		reasonExplicitCleanup:          "explicit-cleanup",
		reasonWrapperPassedToCallee:    "wrapper-passed-to-callee",
		reasonCalleeReleasesArgument:   "callee-releases-argument",
		reasonUnsummarizedCalleeUse:    "unsummarized-callee-uses-resource",
	}
	if len(want) != int(deferReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range deferReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []deferReason{deferReasonCount, 255} {
		if reason.String() != "invalid-defer-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
