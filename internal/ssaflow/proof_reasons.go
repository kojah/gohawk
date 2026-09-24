package ssaflow

var evidenceReasonCodes = [...]string{
	EvidenceNone:                         "",
	EvidenceNotFound:                     "evidence-not-found",
	EvidenceUnavailable:                  "evidence-unavailable",
	EvidenceSameValue:                    "same-value",
	EvidenceSharedSlot:                   "shared-slot",
	EvidenceUnknownPointee:               "unknown-pointee",
	EvidenceDisjointPaths:                "disjoint-paths",
	EvidenceDisjointObjects:              "disjoint-objects",
	EvidenceUnescapedLocal:               "unescaped-local",
	EvidenceStructuralWalk:               "structural-walk",
	EvidenceSameAccessPath:               "same-access-path",
	EvidenceDeferredCompletion:           "deferred-completion",
	EvidenceCalledCompletion:             "called-completion",
	EvidenceStartedCompletion:            "started-completion",
	EvidenceCallbackCompletion:           "callback-completion",
	EvidenceBudgetExhausted:              "budget-exhausted",
	EvidenceCompletionInCycle:            "completion-only-in-cycle",
	EvidenceHelperInvocation:             "helper-invocation",
	EvidenceReturnedDeferredCleanup:      "returned-deferred-cleanup",
	EvidenceStorageNotLocal:              "storage-not-local",
	EvidenceStorageOutsideFunction:       "storage-outside-function",
	EvidenceStorageAddressEscapes:        "storage-address-escapes",
	EvidenceStorageWriteThroughAlias:     "storage-write-through-alias",
	EvidenceStoragePartialWrite:          "storage-partial-write",
	EvidenceStorageConflictingWrites:     "storage-conflicting-writes",
	EvidenceStorageNoReachingWrite:       "storage-no-reaching-write",
	EvidenceStorageWriteInCycle:          "storage-write-in-cycle",
	EvidenceStorageWriteAfterObservation: "storage-write-after-observation",
	EvidenceStorageProjectionNotLoad:     "storage-projection-not-load",
	EvidenceStorageProjectionModified:    "storage-projection-modified",
	EvidenceStoredValuesDiffer:           "stored-values-differ",
	EvidenceSummaryBodyUnavailable:       "summary-body-unavailable",
	EvidenceSummaryRecursive:             "summary-recursive",
	EvidenceStoredInField:                "stored-in-field",
	EvidenceOwnerStoredInField:           "owner-stored-in-field",
	EvidenceStoredInGlobal:               "stored-in-global",
	EvidenceStoredInEnclosingScope:       "stored-in-enclosing-scope",
	EvidenceOwnerStoredInExternalField:   "owner-stored-in-external-field",
	EvidenceStoredInOwnedMap:             "stored-in-owned-map",
	EvidenceSentToReceiver:               "sent-to-receiver",
	EvidenceCapturedByClosure:            "captured-by-closure",
	EvidenceCallResultStoredInField:      "call-result-stored-in-field",
	EvidenceTransferredToReturnedOwner:   "transferred-to-returned-owner",
	EvidenceTransferredToReceiver:        "transferred-to-receiver",
	EvidenceTransferredToLifecycleOwner:  "transferred-to-lifecycle-owner",
	EvidenceCallEffectsKnown:             "call-effects-known",
}

// String renders an evidence classification for tracing or display.
func (reason EvidenceReason) String() string {
	if int(reason) >= len(evidenceReasonCodes) {
		return "invalid-evidence-reason"
	}
	return evidenceReasonCodes[reason]
}
