package resourcelifetime

import "github.com/kojah/gohawk/internal/ssaflow"

// resourceLifetimeReason owns resource evidence and policy classifications.
// Rendering is deliberately separate from the flow and proof decisions.
type resourceLifetimeReason uint8

const (
	resourceReasonNone resourceLifetimeReason = iota
	resourceReasonAcquisitionErrorProven
	resourceReasonAggregateOwnerMayEscape
	resourceReasonAmbiguousCleanupValue
	resourceReasonAmbiguousHelperCleanupValue
	resourceReasonAppended
	resourceReasonBudgetExhausted
	resourceReasonCallEffectsAsynchronousExposure
	resourceReasonCallResultMayTransfer
	resourceReasonDirectMayCarry
	resourceReasonAggregateMayCarry
	resourceReasonWrapperMayCarry
	resourceReasonParentCleanup
	resourceReasonCapturedAggregateOwner
	resourceReasonCapturedBodyGuardedCleanup
	resourceReasonCapturedByLiteralCallingUnreadableCallee
	resourceReasonCapturedByPossiblyRetainedCallback
	resourceReasonCapturedByPriorCleanup
	resourceReasonCapturedByRetainingLiteral
	resourceReasonCapturedByStartedLiteral
	resourceReasonCapturedCellMayCleanup
	resourceReasonCompressionOutputMayBeAbandoned
	resourceReasonCanceledAcquisition
	resourceReasonDynamicCallee
	resourceReasonErrorPredicateFalseForNil
	resourceReasonErrorTypeAssertionSucceeded
	resourceReasonErrorsAsExactAcquisitionError
	resourceReasonErrorsIsNonNilFilesystemSentinel
	resourceReasonErrorsIsNonNilContextSentinel
	resourceReasonEvidenceNotFound
	resourceReasonEvidenceUnavailable
	resourceReasonExactErrorEqualsNonNilFilesystemSentinel
	resourceReasonHeadAcquisition
	resourceReasonHeadClientNotUnconfigured
	resourceReasonHeadRequestModified
	resourceReasonHelperCleanupInLoop
	resourceReasonImportedHelperCleanupInLoop
	resourceReasonInterfaceMethod
	resourceReasonHeaderOnlyAcquisition
	resourceReasonLocalServerClientOverrideUnresolved
	resourceReasonLocalServerHandlerUnavailable
	resourceReasonLocalServerHeaderOnlyEffects
	resourceReasonLocalServerIdentityUnavailable
	resourceReasonLocalServerWriterEffectsUnavailable
	resourceReasonNestedInTransferredArgument
	resourceReasonUntouched
	resourceReasonOpaqueConsumption
	optionalAcquisitionSuccessPhi
	resourceReasonPairedErrorHelperCleanup
	resourceReasonPriorDeferMayCleanCapturedCell
	resourceReasonReleaseProven
	resourceReasonRepeatedGuardEdgeUnknown
	resourceReasonResourceReturnPath
	resourceReasonReturnedCleanupProjection
	resourceReasonReturnedMayTransfer
	resourceReasonReturnedWrapperRetains
	resourceReasonReturnedProjectionLacksCleanup
	resourceReasonReturnedViewCannotRelease
	resourceReasonRowsTransactionFinished
	resourceReasonTransactionContextCanceled
	resourceReasonSentToChannel
	resourceReasonResponseBodyAggregateHandoff
	resourceReasonSettled
	resourceReasonStatementParentClosed
	resourceReasonStoredInMap
	resourceReasonStoredOnCollectionOwner
	resourceReasonIndirectDestinationUnknown
	resourceReasonWrapperStoredOnForeignOwner
	resourceReasonAcquisitionUnreachable
	resourceReasonTestifyNoErrorGuard
	resourceReasonUnownedReturn
	resourceReasonUnsummarizedCallee
	resourceReasonOSIsNotExist
	resourceReasonOSIsExist
	resourceReasonOSIsPermission
	resourceReasonOSIsTimeout
	resourceReasonProcessExitReclaims
	resourceReasonReturnedRetainingWrapper
	resourceReasonAppendedToLocalCollection
	resourceReasonCollectionReturned
	resourceReasonCollectionReleasedByHelper
	resourceReasonCollectionReleased
	resourceReasonCollectionUseUnknown
	resourceReasonRowsExhaustedEdgeUnknown
	resourceReasonResultGuardedDefer
	resourceReasonResultGuardedRelease
	resourceReasonResultGuardedUnknown
	resourceReasonMemoryWriter
	resourceReasonAcquisitionLocationUnknown
	resourceReasonPriorDeferMayRelease
	resourceReasonCount
)

// These codes are a trace compatibility boundary, never proof inputs. In
// particular, unset ("") and an inspected-but-untouched instruction ("none")
// stay distinct. Numeric ordinals can change without changing that contract.
var resourceReasonCodes = [...]string{
	resourceReasonDirectMayCarry:                           "direct-value-may-carry-resource",
	resourceReasonAggregateMayCarry:                        "aggregate-may-carry-resource",
	resourceReasonWrapperMayCarry:                          "wrapper-may-carry-resource",
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
	resourceReasonCallResultMayTransfer:                    "call-result-may-transfer",
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

func (reason resourceLifetimeReason) String() string {
	if int(reason) >= len(resourceReasonCodes) {
		return "invalid-resource-reason"
	}
	return resourceReasonCodes[reason]
}

// resourceProof records analyzer-owned evidence. It does not inject resource
// API contracts into the shared SSA reason domain.
type resourceProof struct {
	State      ssaflow.EvidenceState
	Reason     resourceLifetimeReason
	Provenance ssaflow.EvidenceProvenance
}

func (proof resourceProof) Proven() bool { return proof.State == ssaflow.EvidenceProven }

// within discards query evidence if its child allowance or shared pool ran out.
// Apply it after the authoritative query; an early witness is not a completed
// proof when later work was cut. Available proofs keep their domain meaning.
func (proof resourceProof) within(budget *ssaflow.SearchBudget) resourceProof {
	if resourceFlowExhausted(budget) {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	return proof
}
