package lockorder

// lockReason explains lock identity, release, and ordering evidence.
type lockReason uint8

const (
	lockReasonNone lockReason = iota
	lockReasonCarriedConstantBranchInfeasible
	lockReasonConditionalCallerReleaseProven
	lockReasonConditionalCallerReleaseUnknown
	lockReasonCrossOwnerClassUnknown
	lockReasonDeferredReleaseProven
	lockReasonCycleOrderRecorded
	lockReasonExclusiveObjectBeforePublication
	lockReasonExclusiveParameterFromFreshCallers
	lockReasonFreshBoundOwnerIdentityUnknown
	lockReasonFreshFieldIdentityUnknown
	lockReasonImportedWriterGuardUnknown
	lockReasonLoadedAcquisitionGuardUnknown
	lockReasonLockStateBudgetExhausted
	lockReasonHelperReleaseUnproven
	lockReasonHeldForCallerProven
	lockReasonHeldForCallerUnknown
	lockReasonMutexActionObserved
	lockReasonReleaseIdentityUnknown
	lockReasonNoFreshBoundOwner
	lockReasonNoFreshFieldWitness
	lockReasonOppositeOrderRecorded
	lockReasonPredecessorConstantBranchInfeasible
	lockReasonRepeatedConditionInfeasible
	lockReasonStableParameterBranchInfeasible
	lockReasonPrivateMutexOnly
	lockReasonReleaseOwnershipUnknown
	lockReasonUnreleasedReturn
	lockReasonReadLockWrite
	lockReasonExclusiveWriterUnknown
	lockReasonPrivateWriteStorage
	lockReasonFieldGuardUnknown
	lockReasonInitialPublicationUnknown
	lockReasonCount
)

var lockReasonCodes = [...]string{
	lockReasonInitialPublicationUnknown:           "initial-publication-order-unknown",
	lockReasonFieldGuardUnknown:                   "field-guard-unknown",
	lockReasonPrivateWriteStorage:                 "private-write-storage",
	lockReasonPrivateMutexOnly:                    "private-mutex-only",
	lockReasonReleaseOwnershipUnknown:             "release-ownership-unknown",
	lockReasonUnreleasedReturn:                    "unreleased-return",
	lockReasonReadLockWrite:                       "read-lock-write",
	lockReasonExclusiveWriterUnknown:              "exclusive-writer-guard-unknown",
	lockReasonNone:                                "",
	lockReasonCarriedConstantBranchInfeasible:     "carried-constant-branch-infeasible",
	lockReasonConditionalCallerReleaseProven:      "conditional-caller-release-proven",
	lockReasonConditionalCallerReleaseUnknown:     "conditional-caller-release-unknown",
	lockReasonCrossOwnerClassUnknown:              "cross-owner-class-unknown",
	lockReasonDeferredReleaseProven:               "deferred-release-proven",
	lockReasonCycleOrderRecorded:                  "cycle-order-recorded",
	lockReasonExclusiveObjectBeforePublication:    "exclusive-object-before-publication",
	lockReasonExclusiveParameterFromFreshCallers:  "exclusive-parameter-from-fresh-callers",
	lockReasonFreshBoundOwnerIdentityUnknown:      "fresh-bound-owner-identity-unknown",
	lockReasonFreshFieldIdentityUnknown:           "fresh-field-identity-unknown",
	lockReasonImportedWriterGuardUnknown:          "imported-writer-guard-unknown",
	lockReasonLoadedAcquisitionGuardUnknown:       "loaded-acquisition-guard-unknown",
	lockReasonLockStateBudgetExhausted:            "lock-state-budget-exhausted",
	lockReasonHelperReleaseUnproven:               "helper-release-unproven",
	lockReasonHeldForCallerProven:                 "held-for-caller-proven",
	lockReasonHeldForCallerUnknown:                "held-for-caller-unknown",
	lockReasonMutexActionObserved:                 "mutex-action-observed",
	lockReasonReleaseIdentityUnknown:              "release-identity-unknown",
	lockReasonNoFreshBoundOwner:                   "no-fresh-bound-owner",
	lockReasonNoFreshFieldWitness:                 "no-fresh-field-witness",
	lockReasonOppositeOrderRecorded:               "opposite-order-recorded",
	lockReasonPredecessorConstantBranchInfeasible: "predecessor-constant-branch-infeasible",
	lockReasonRepeatedConditionInfeasible:         "repeated-condition-infeasible",
	lockReasonStableParameterBranchInfeasible:     "stable-parameter-branch-infeasible",
}

func (reason lockReason) String() string {
	if int(reason) >= len(lockReasonCodes) {
		return "invalid-lock-reason"
	}
	return lockReasonCodes[reason]
}
