package resourcelifetime

import "testing"

// resourceReasonSpellings is every reason's code as a trace prints it.
var resourceReasonSpellings = map[resourceLifetimeReason]string{
	resourceReasonMemoryWriter:                             "memory-writer-no-external-resource",
	resourceReasonAcquisitionLocationUnknown:               "acquisition-location-unknown",
	resourceReasonPriorDeferMayRelease:                     "prior-defer-may-release",
	resourceReasonNone:                                     "",
	resourceReasonAcquisitionErrorProven:                   "acquisition-error-proven",
	resourceReasonAggregateOwnerMayEscape:                  "aggregate-owner-may-escape",
	resourceReasonAmbiguousCleanupValue:                    "ambiguous-cleanup-value",
	resourceReasonAmbiguousHelperCleanupValue:              "ambiguous-helper-cleanup-value",
	resourceReasonAppended:                                 "appended",
	resourceReasonAppendedToLocalCollection:                "appended-to-local-collection",
	resourceReasonCollectionReturned:                       "collection-returned",
	resourceReasonCollectionReleasedByHelper:               "collection-released-by-helper",
	resourceReasonCollectionReleased:                       "collection-released",
	resourceReasonCollectionUseUnknown:                     "collection-use-unknown",
	resourceReasonRowsExhaustedEdgeUnknown:                 "rows-exhausted-edge-unknown",
	resourceReasonResultGuardedDefer:                       "result-guarded-defer",
	resourceReasonResultGuardedRelease:                     "result-guarded-release",
	resourceReasonResultGuardedUnknown:                     "result-guarded-unknown",
	resourceReasonBudgetExhausted:                          "budget-exhausted",
	resourceReasonCallEffectsAsynchronousExposure:          "call-effects-asynchronous-exposure",
	resourceReasonParentCleanup:                            "caller-owned-database-cleanup",
	resourceReasonCapturedAggregateOwner:                   "captured-aggregate-owner",
	resourceReasonCapturedBodyGuardedCleanup:               "captured-body-guarded-cleanup",
	resourceReasonCapturedByLiteralCallingUnreadableCallee: "captured-by-literal-calling-unreadable-callee",
	resourceReasonCapturedByPossiblyRetainedCallback:       "captured-by-possibly-retained-callback",
	resourceReasonCapturedByPriorCleanup:                   "captured-by-prior-cleanup",
	resourceReasonCapturedByRetainingLiteral:               "captured-by-retaining-literal",
	resourceReasonCapturedByStartedLiteral:                 "captured-by-started-literal",
	resourceReasonCapturedCellMayCleanup:                   "captured-cell-may-cleanup",
	resourceReasonCompressionOutputMayBeAbandoned:          "compression-output-may-be-abandoned",
	resourceReasonCanceledAcquisition:                      "context-canceled-before-acquisition",
	resourceReasonDynamicCallee:                            "dynamic-callee",
	resourceReasonErrorPredicateFalseForNil:                "error-predicate-false-for-nil",
	resourceReasonErrorTypeAssertionSucceeded:              "error-type-assertion-succeeded",
	resourceReasonErrorsAsExactAcquisitionError:            "errors-as-exact-acquisition-error",
	resourceReasonErrorsIsNonNilContextSentinel:            "errors-is-non-nil-context-sentinel",
	resourceReasonErrorsIsNonNilFilesystemSentinel:         "errors-is-non-nil-filesystem-sentinel",
	resourceReasonEvidenceNotFound:                         "evidence-not-found",
	resourceReasonEvidenceUnavailable:                      "evidence-unavailable",
	resourceReasonExactErrorEqualsNonNilFilesystemSentinel: "exact-error-equals-non-nil-filesystem-sentinel",
	resourceReasonHeadAcquisition:                          "head-body-acquisition-uncertain",
	resourceReasonHeadClientNotUnconfigured:                "head-client-not-unconfigured",
	resourceReasonHeadRequestModified:                      "head-request-modified",
	resourceReasonHelperCleanupInLoop:                      "helper-cleanup-in-loop",
	resourceReasonImportedHelperCleanupInLoop:              "imported-helper-cleanup-in-loop",
	resourceReasonInterfaceMethod:                          "interface-method",
	resourceReasonHeaderOnlyAcquisition:                    "local-header-only-body-uncertain",
	resourceReasonLocalServerClientOverrideUnresolved:      "local-server-client-override-unresolved",
	resourceReasonLocalServerHandlerUnavailable:            "local-server-handler-unavailable",
	resourceReasonLocalServerHeaderOnlyEffects:             "local-server-header-only-effects",
	resourceReasonLocalServerIdentityUnavailable:           "local-server-identity-unavailable",
	resourceReasonLocalServerWriterEffectsUnavailable:      "local-server-writer-effects-unavailable",
	resourceReasonNestedInTransferredArgument:              "nested-in-transferred-argument",
	resourceReasonUntouched:                                "none",
	resourceReasonOpaqueConsumption:                        "opaque-consumption",
	optionalAcquisitionSuccessPhi:                          "optional-acquisition-success-phi",
	resourceReasonPairedErrorHelperCleanup:                 "paired-error-helper-cleanup",
	resourceReasonPriorDeferMayCleanCapturedCell:           "prior-defer-may-clean-captured-cell",
	resourceReasonReleaseProven:                            "release-proven",
	resourceReasonRepeatedGuardEdgeUnknown:                 "repeated-guard-edge-unknown",
	resourceReasonResourceReturnPath:                       "resource-return-path",
	resourceReasonReturnedCleanupProjection:                "returned-cleanup-projection",
	resourceReasonReturnedMayTransfer:                      "returned-may-transfer",
	resourceReasonReturnedWrapperRetains:                   "returned-wrapper-retains-resource",
	resourceReasonReturnedProjectionLacksCleanup:           "returned-projection-lacks-cleanup",
	resourceReasonReturnedViewCannotRelease:                "returned-view-cannot-release",
	resourceReasonRowsTransactionFinished:                  "rows-transaction-finished",
	resourceReasonTransactionContextCanceled:               "transaction-context-canceled",
	resourceReasonResponseBodyAggregateHandoff:             "response-body-aggregate-handoff",
	resourceReasonSentToChannel:                            "sent-to-channel",
	resourceReasonSettled:                                  "settled",
	resourceReasonStatementParentClosed:                    "statement-parent-closed",
	resourceReasonStoredInMap:                              "stored-in-map",
	resourceReasonIndirectDestinationUnknown:               "indirect-destination-unknown",
	resourceReasonStoredOnCollectionOwner:                  "stored-on-collection-owner",
	resourceReasonWrapperStoredOnForeignOwner:              "wrapper-stored-on-foreign-owner",
	resourceReasonAcquisitionUnreachable:                   "acquisition-unreachable",
	resourceReasonTestifyNoErrorGuard:                      "testify-no-error-guard",
	resourceReasonUnownedReturn:                            "unowned-return",
	resourceReasonUnsummarizedCallee:                       "unsummarized-callee",
	resourceReasonOSIsNotExist:                             "os-isnotexist",
	resourceReasonOSIsExist:                                "os-isexist",
	resourceReasonOSIsPermission:                           "os-ispermission",
	resourceReasonOSIsTimeout:                              "os-istimeout",
	resourceReasonProcessExitReclaims:                      "process-exit-reclaims",
	resourceReasonReturnedRetainingWrapper:                 "returned-retaining-wrapper",
}

func TestResourceLifetimeReasonCodes(t *testing.T) {
	want := resourceReasonSpellings
	if len(want) != int(resourceReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range resourceReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []resourceLifetimeReason{resourceReasonCount, 255} {
		if reason.String() != "invalid-resource-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
