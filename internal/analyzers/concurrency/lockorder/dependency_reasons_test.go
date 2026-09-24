package lockorder

import "testing"

func TestDependencyReasonCodes(t *testing.T) {
	want := map[dependencyReason]string{
		dependencyNone:                                "",
		dependencyChannelLockCandidate:                "channel-lock-candidate",
		dependencyChannelLockCapacityUnknown:          "channel-lock-capacity-unknown",
		dependencyChannelLockCycleProven:              "channel-lock-cycle-proven",
		dependencyLockJoinAlternateUnlock:             "lock-join-alternate-unlock",
		dependencyLockJoinAlternativeSitesDiffer:      "lock-join-alternative-sites-differ",
		dependencyLockJoinCandidate:                   "lock-join-candidate",
		dependencyLockJoinCompletionUnknown:           "lock-join-completion-unknown",
		dependencyLockJoinCycleUnproven:               "lock-join-cycle-unproven",
		dependencyLockJoinDeadlockProven:              "lock-join-deadlock-proven",
		dependencyLockJoinIdentityUnknown:             "lock-join-identity-unknown",
		dependencyLockJoinParentOrderNotMatched:       "lock-join-parent-order-not-matched",
		dependencyLockJoinShapeNotMatched:             "lock-join-shape-not-matched",
		dependencyLockJoinSignalKindNotMatched:        "lock-join-signal-kind-not-matched",
		dependencyLockJoinSourceUnknown:               "lock-join-source-unknown",
		dependencyLockJoinSpawnOrderUnknown:           "lock-join-spawn-order-unknown",
		dependencyLockJoinWorkerIdentityUnknown:       "lock-join-worker-identity-unknown",
		dependencyLockJoinWorkerOrderNotMatched:       "lock-join-worker-order-not-matched",
		dependencySyncCycleCandidate:                  "sync-cycle-candidate",
		dependencyWaitgroupLockAlternativeSitesDiffer: "waitgroup-lock-alternative-sites-differ",
		dependencyWaitgroupLockCandidate:              "waitgroup-lock-candidate",
		dependencyWaitgroupLockCounterUnknown:         "waitgroup-lock-counter-unknown",
		dependencyWaitgroupLockCycleProven:            "waitgroup-lock-cycle-proven",
		dependencyWaitgroupLockCycleUnproven:          "waitgroup-lock-cycle-unproven",
		dependencyWaitgroupLockParentOrderNotMatched:  "waitgroup-lock-parent-order-not-matched",
		dependencyWaitgroupLockShapeNotMatched:        "waitgroup-lock-shape-not-matched",
		dependencyWaitgroupLockSourceUnknown:          "waitgroup-lock-source-unknown",
		dependencyWaitgroupLockWorkerEffectsUnknown:   "waitgroup-lock-worker-effects-unknown",
		dependencyWaitgroupLockWorkerOrderNotMatched:  "waitgroup-lock-worker-order-not-matched",
	}
	if len(want) != int(dependencyReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range dependencyReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []dependencyReason{dependencyReasonCount, 255} {
		if reason.String() != "invalid-lockorder-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
