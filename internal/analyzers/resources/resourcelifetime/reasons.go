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
	resourceReasonEvidenceNotFound
	resourceReasonEvidenceUnavailable
	resourceReasonExactErrorEqualsNonNilFilesystemSentinel
	resourceReasonHeadAcquisition
	resourceReasonHeadClientNotUnconfigured
	resourceReasonHeadRequestModified
	resourceReasonHelperCleanupInLoop
	resourceReasonHelperRequiresOperation
	resourceReasonImportedHelperCleanupInLoop
	resourceReasonInterfaceMethod
	resourceReasonKnownResourceDirectRelease
	resourceReasonKnownResourceHelperRelease
	resourceReasonManifestReleasedUse
	resourceReasonLatentReleasedUse
	resourceReasonHeaderOnlyAcquisition
	resourceReasonLocalServerClientOverrideUnresolved
	resourceReasonLocalServerHandlerUnavailable
	resourceReasonLocalServerHeaderOnlyEffects
	resourceReasonLocalServerIdentityUnavailable
	resourceReasonLocalServerWriterEffectsUnavailable
	resourceReasonNestedInTransferredArgument
	resourceReasonUntouched
	resourceReasonOpaqueConsumption
	resourceReasonOperationOnReleasedResource
	optionalAcquisitionSuccessPhi
	resourceReasonPairedErrorHelperCleanup
	resourceReasonPriorDeferMayCleanCapturedCell
	resourceReasonReleaseDoesNotDominateUse
	resourceReasonReleaseDominatesUse
	resourceReasonReleaseProven
	resourceReasonReleaseSuccessNotProven
	resourceReasonReleaseUseBudgetExhausted
	resourceReasonReleaseUseOpaqueEffect
	resourceReasonReleaseUseUnreachable
	resourceReasonRepeatedGuardEdgeUnknown
	resourceReasonResourceReturnPath
	resourceReasonReturnedCleanupProjection
	resourceReasonReturnedWrapperRetains
	resourceReasonReturnedProjectionLacksCleanup
	resourceReasonReturnedViewCannotRelease
	resourceReasonRowsTransactionFinished
	resourceReasonSentToChannel
	resourceReasonSettled
	resourceReasonStatementParentClosed
	resourceReasonStoredInMap
	resourceReasonStoredOnCollectionOwner
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
	resourceReasonCount
)

// These codes are a trace compatibility boundary, never proof inputs. In
// particular, unset ("") and an inspected-but-untouched instruction ("none")
// stay distinct. Numeric ordinals can change without changing that contract.
var resourceReasonCodes = [...]string{
	resourceReasonNone:                                     "",
	resourceReasonAcquisitionErrorProven:                   "acquisition-error-proven",
	resourceReasonAggregateOwnerMayEscape:                  "aggregate-owner-may-escape",
	resourceReasonAmbiguousCleanupValue:                    "ambiguous-cleanup-value",
	resourceReasonAmbiguousHelperCleanupValue:              "ambiguous-helper-cleanup-value",
	resourceReasonAppended:                                 "appended",
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
	resourceReasonErrorsIsNonNilFilesystemSentinel:         "errors-is-non-nil-filesystem-sentinel",
	resourceReasonEvidenceNotFound:                         "evidence-not-found",
	resourceReasonEvidenceUnavailable:                      "evidence-unavailable",
	resourceReasonExactErrorEqualsNonNilFilesystemSentinel: "exact-error-equals-non-nil-filesystem-sentinel",
	resourceReasonHeadAcquisition:                          "head-body-acquisition-uncertain",
	resourceReasonHeadClientNotUnconfigured:                "head-client-not-unconfigured",
	resourceReasonHeadRequestModified:                      "head-request-modified",
	resourceReasonHelperCleanupInLoop:                      "helper-cleanup-in-loop",
	resourceReasonHelperRequiresOperation:                  "helper-requires-operation",
	resourceReasonImportedHelperCleanupInLoop:              "imported-helper-cleanup-in-loop",
	resourceReasonInterfaceMethod:                          "interface-method",
	resourceReasonKnownResourceDirectRelease:               "known-resource-direct-release",
	resourceReasonKnownResourceHelperRelease:               "known-resource-helper-release",
	resourceReasonManifestReleasedUse:                      "manifest-released-use",
	resourceReasonLatentReleasedUse:                        "latent-released-use",
	resourceReasonHeaderOnlyAcquisition:                    "local-header-only-body-uncertain",
	resourceReasonLocalServerClientOverrideUnresolved:      "local-server-client-override-unresolved",
	resourceReasonLocalServerHandlerUnavailable:            "local-server-handler-unavailable",
	resourceReasonLocalServerHeaderOnlyEffects:             "local-server-header-only-effects",
	resourceReasonLocalServerIdentityUnavailable:           "local-server-identity-unavailable",
	resourceReasonLocalServerWriterEffectsUnavailable:      "local-server-writer-effects-unavailable",
	resourceReasonNestedInTransferredArgument:              "nested-in-transferred-argument",
	resourceReasonUntouched:                                "none",
	resourceReasonOpaqueConsumption:                        "opaque-consumption",
	resourceReasonOperationOnReleasedResource:              "operation-on-released-resource",
	optionalAcquisitionSuccessPhi:                          "optional-acquisition-success-phi",
	resourceReasonPairedErrorHelperCleanup:                 "paired-error-helper-cleanup",
	resourceReasonPriorDeferMayCleanCapturedCell:           "prior-defer-may-clean-captured-cell",
	resourceReasonReleaseDoesNotDominateUse:                "release-does-not-dominate-use",
	resourceReasonReleaseDominatesUse:                      "release-dominates-use",
	resourceReasonReleaseProven:                            "release-proven",
	resourceReasonReleaseSuccessNotProven:                  "release-success-not-proven",
	resourceReasonReleaseUseBudgetExhausted:                "release-use-budget-exhausted",
	resourceReasonReleaseUseOpaqueEffect:                   "release-use-opaque-effect",
	resourceReasonReleaseUseUnreachable:                    "release-use-unreachable",
	resourceReasonRepeatedGuardEdgeUnknown:                 "repeated-guard-edge-unknown",
	resourceReasonResourceReturnPath:                       "resource-return-path",
	resourceReasonReturnedCleanupProjection:                "returned-cleanup-projection",
	resourceReasonReturnedWrapperRetains:                   "returned-wrapper-retains-resource",
	resourceReasonReturnedProjectionLacksCleanup:           "returned-projection-lacks-cleanup",
	resourceReasonReturnedViewCannotRelease:                "returned-view-cannot-release",
	resourceReasonRowsTransactionFinished:                  "rows-transaction-finished",
	resourceReasonSentToChannel:                            "sent-to-channel",
	resourceReasonSettled:                                  "settled",
	resourceReasonStatementParentClosed:                    "statement-parent-closed",
	resourceReasonStoredInMap:                              "stored-in-map",
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
