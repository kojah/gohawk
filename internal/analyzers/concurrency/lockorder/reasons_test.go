package lockorder

import "testing"

func TestLockReasonCodes(t *testing.T) {
	want := map[lockReason]string{
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
