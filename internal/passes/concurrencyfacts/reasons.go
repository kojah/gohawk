package concurrencyfacts

// Reason is the summary component's closed failure and observation vocabulary.
// A zero reason records no cutoff; completeness still checks paths and bindings.
// Consumers retain this enum through composition and format it only at output.
type Reason uint8

const (
	ReasonNone Reason = iota
	ReasonComponentNotRequested
	ReasonComponentUnavailable
	ReasonAlternativeLimit
	ReasonBodyUnavailable
	ReasonBranchAlternatives
	ReasonBranchEffectsDiffer
	ReasonBudgetExhausted
	ReasonChannelBindingUnknown
	ReasonChannelIdentityUnknown
	ReasonCondLockerUnknown
	ReasonContextBindingRequired
	ReasonContextBindingUnknown
	ReasonContextIdentityUnknown
	ReasonContextParentUnknown
	ReasonControlFlowUnknown
	ReasonCutoff
	ReasonDeferredEffectsUnknown
	ReasonEffectUnknown
	ReasonFieldBindingUnknown
	ReasonGroupCountUnknown
	ReasonLoadUnknown
	ReasonLocalContextUnknown
	ReasonParticipantsUnknown
	ReasonPayloadUnknown
	ReasonSelectAlternatives
	ReasonSelectAlternativesUnknown
	ReasonSelectDispatchUnknown
	ReasonSelectNoFeasibleArm
	ReasonSummaryLimit
	ReasonWorkerEffectsUnknown
	ReasonSummarizing
	ReasonExportUnknown
	ReasonExportComplete
	ReasonRecursiveProtocol
	reasonCount
)

var reasonCodes = [...]string{
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

// String is the stable trace representation; numeric values are not wire codes.
func (reason Reason) String() string {
	if int(reason) >= len(reasonCodes) {
		return "invalid-concurrency-reason"
	}
	return reasonCodes[reason]
}
