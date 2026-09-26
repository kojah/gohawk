package cancellationownership

// Cancellation reasons classify the authoritative obligation proof. Formatting
// belongs at the trace boundary and does not determine the proof's outcome.
type cancellationReason uint8

const (
	reasonCancellationNone cancellationReason = iota
	reasonCancellationUnknown
	reasonCancellationReleased
	reasonCancellationTransferred
	reasonCancellationLost
	// The label reasons name why the classifier labelled one instruction.
	reasonLabelRelease
	reasonLabelOpaqueUse
	reasonLabelParentContextUse
	reasonLabelLaunchedCancel
	reasonLabelOwnDoneReceive
	reasonLabelDeferredClosure
	reasonLabelReturnedCallback
	reasonLabelTestingCleanup
	reasonLabelRegisteredCallback
	reasonLabelLaunchedHelper
	reasonLabelHelperRelease
	reasonLabelSummaryRelease
	reasonLabelHelperUndecided
	reasonLabelHelperMayInvoke
	reasonLabelReturnsDeferredCleanup
	reasonLabelCapturedByCallback
	reasonLabelStored
	reasonLabelSent
	reasonLabelStoredInMap
	reasonLabelPassedToCallee
	reasonLabelReturned
	reasonLabelAliased
	reasonLabelResultGuardedDefer
	reasonLabelDeferredLiteralRelease
	reasonLabelResultGuardedRelease
	reasonLabelResultGuardedUnknown
	cancellationReasonCount
)

var cancellationReasonCodes = [...]string{
	reasonCancellationNone:            "",
	reasonCancellationUnknown:         "ambiguous-cancellation-use",
	reasonCancellationReleased:        "exact-cancellation-release",
	reasonCancellationTransferred:     "exact-cancellation-transfer",
	reasonCancellationLost:            "unowned-return",
	reasonLabelRelease:                "release",
	reasonLabelOpaqueUse:              "opaque-cancellation-use",
	reasonLabelParentContextUse:       "parent-context-use",
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
	reasonLabelPassedToCallee:         "passed-to-callee",
	reasonLabelReturned:               "returned",
	reasonLabelAliased:                "aliased",
	reasonLabelResultGuardedDefer:     "result-guarded-defer",
	reasonLabelDeferredLiteralRelease: "deferred-literal-release",
	reasonLabelResultGuardedRelease:   "result-guarded-release",
	reasonLabelResultGuardedUnknown:   "result-guarded-unknown",
}

func (reason cancellationReason) String() string {
	if int(reason) >= len(cancellationReasonCodes) {
		return "invalid-cancellation-reason"
	}
	return cancellationReasonCodes[reason]
}
