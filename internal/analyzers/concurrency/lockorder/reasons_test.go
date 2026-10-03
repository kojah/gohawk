package lockorder

import "testing"

func TestLockReasonCodes(t *testing.T) {
	want := map[lockReason]string{
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
	if len(want) != int(lockReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range lockReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []lockReason{lockReasonCount, 255} {
		if reason.String() != "invalid-lock-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
