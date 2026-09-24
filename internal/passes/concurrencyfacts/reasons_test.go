package concurrencyfacts

import "testing"

func TestReasonCodes(t *testing.T) {
	want := map[Reason]string{
		ReasonNone:                      "",
		ReasonComponentNotRequested:     "concurrency-component-not-requested",
		ReasonComponentUnavailable:      "concurrency-component-unavailable",
		ReasonAlternativeLimit:          "protocol-alternative-limit",
		ReasonBodyUnavailable:           "protocol-body-unavailable",
		ReasonBranchAlternatives:        "protocol-branch-alternatives",
		ReasonBranchEffectsDiffer:       "protocol-branch-effects-differ",
		ReasonBudgetExhausted:           "protocol-budget-exhausted",
		ReasonChannelBindingUnknown:     "protocol-channel-binding-unknown",
		ReasonChannelIdentityUnknown:    "protocol-channel-identity-unknown",
		ReasonCondLockerUnknown:         "protocol-cond-locker-unknown",
		ReasonContextBindingRequired:    "protocol-context-binding-required",
		ReasonContextBindingUnknown:     "protocol-context-binding-unknown",
		ReasonContextIdentityUnknown:    "protocol-context-identity-unknown",
		ReasonContextParentUnknown:      "protocol-context-parent-unknown",
		ReasonControlFlowUnknown:        "protocol-control-flow-unknown",
		ReasonCutoff:                    "protocol-cutoff",
		ReasonDeferredEffectsUnknown:    "protocol-deferred-effects-unknown",
		ReasonEffectUnknown:             "protocol-effect-unknown",
		ReasonFieldBindingUnknown:       "protocol-field-binding-unknown",
		ReasonGroupCountUnknown:         "protocol-group-count-unknown",
		ReasonLoadUnknown:               "protocol-load-unknown",
		ReasonLocalContextUnknown:       "protocol-local-context-unknown",
		ReasonParticipantsUnknown:       "protocol-participants-unknown",
		ReasonPayloadUnknown:            "protocol-payload-unknown",
		ReasonSelectAlternatives:        "protocol-select-alternatives",
		ReasonSelectAlternativesUnknown: "protocol-select-alternatives-unknown",
		ReasonSelectDispatchUnknown:     "protocol-select-dispatch-unknown",
		ReasonSelectNoFeasibleArm:       "protocol-select-no-feasible-arm",
		ReasonSummaryLimit:              "protocol-summary-limit",
		ReasonWorkerEffectsUnknown:      "protocol-worker-effects-unknown",
		ReasonSummarizing:               "summarizing-concurrency",
		ReasonExportUnknown:             "concurrency-export-unknown",
		ReasonExportComplete:            "concurrency-export-complete",
		ReasonRecursiveProtocol:         "recursive-protocol",
	}
	if len(want) != int(reasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range reasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []Reason{reasonCount, 255} {
		if reason.String() != "invalid-concurrency-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
