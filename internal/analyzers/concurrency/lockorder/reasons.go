package lockorder

// lockReason explains lock identity, release, and ordering evidence.
type lockReason uint8

const (
	lockReasonNone lockReason = iota
	lockReasonCarriedConstantBranchInfeasible
	lockReasonConditionalCallerReleaseProven
	lockReasonConditionalCallerReleaseUnknown
	lockReasonCrossOwnerClassUnknown
	lockReasonCycleOrderRecorded
	lockReasonExclusiveObjectBeforePublication
	lockReasonExclusiveParameterFromFreshCallers
	lockReasonFreshBoundOwnerIdentityUnknown
	lockReasonFreshFieldIdentityUnknown
	lockReasonImportedWriterGuardUnknown
	lockReasonLoadedAcquisitionGuardUnknown
	lockReasonLoadedLoopReleaseUnknown
	lockReasonLockStateBudgetExhausted
	lockReasonNoFreshBoundOwner
	lockReasonNoFreshFieldWitness
	lockReasonNoMatchingLoadedLoopRelease
	lockReasonOppositeOrderRecorded
	lockReasonPredecessorConstantBranchInfeasible
	lockReasonRepeatedConditionInfeasible
	lockReasonStableParameterBranchInfeasible
	lockReasonCount
)

var lockReasonCodes = [...]string{
	lockReasonNone: "",
	lockReasonCarriedConstantBranchInfeasible:     "carried-constant-branch-infeasible",
	lockReasonConditionalCallerReleaseProven:      "conditional-caller-release-proven",
	lockReasonConditionalCallerReleaseUnknown:     "conditional-caller-release-unknown",
	lockReasonCrossOwnerClassUnknown:              "cross-owner-class-unknown",
	lockReasonCycleOrderRecorded:                  "cycle-order-recorded",
	lockReasonExclusiveObjectBeforePublication:    "exclusive-object-before-publication",
	lockReasonExclusiveParameterFromFreshCallers:  "exclusive-parameter-from-fresh-callers",
	lockReasonFreshBoundOwnerIdentityUnknown:      "fresh-bound-owner-identity-unknown",
	lockReasonFreshFieldIdentityUnknown:           "fresh-field-identity-unknown",
	lockReasonImportedWriterGuardUnknown:          "imported-writer-guard-unknown",
	lockReasonLoadedAcquisitionGuardUnknown:       "loaded-acquisition-guard-unknown",
	lockReasonLoadedLoopReleaseUnknown:            "loaded-loop-release-unknown",
	lockReasonLockStateBudgetExhausted:            "lock-state-budget-exhausted",
	lockReasonNoFreshBoundOwner:                   "no-fresh-bound-owner",
	lockReasonNoFreshFieldWitness:                 "no-fresh-field-witness",
	lockReasonNoMatchingLoadedLoopRelease:         "no-matching-loaded-loop-release",
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
