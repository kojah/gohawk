package lockorder

// dependencyReason classifies this check's evidence independently of upstream model failures.
type dependencyReason uint8

const (
	dependencyNone dependencyReason = iota
	dependencyChannelLockCandidate
	dependencyChannelLockCapacityUnknown
	dependencyChannelLockCycleProven
	dependencyLockJoinAlternateUnlock
	dependencyLockJoinAlternativeSitesDiffer
	dependencyLockJoinCandidate
	dependencyLockJoinCompletionUnknown
	dependencyLockJoinCycleUnproven
	dependencyLockJoinDeadlockProven
	dependencyLockJoinIdentityUnknown
	dependencyLockJoinParentOrderNotMatched
	dependencyLockJoinPathsInfeasible
	dependencyLockJoinPathsUnknown
	dependencyLockJoinShapeNotMatched
	dependencyLockJoinSignalKindNotMatched
	dependencyLockJoinSourceUnknown
	dependencyLockJoinSpawnOrderUnknown
	dependencyLockJoinWorkerIdentityUnknown
	dependencyLockJoinWorkerOrderNotMatched
	dependencySyncCycleCandidate
	dependencyWaitgroupLockAlternativeSitesDiffer
	dependencyWaitgroupLockCandidate
	dependencyWaitgroupLockCounterUnknown
	dependencyWaitgroupLockCycleProven
	dependencyWaitgroupLockCycleUnproven
	dependencyWaitgroupLockParentOrderNotMatched
	dependencyWaitgroupLockPathsInfeasible
	dependencyWaitgroupLockPathsUnknown
	dependencyWaitgroupLockShapeNotMatched
	dependencyWaitgroupLockSourceUnknown
	dependencyWaitgroupLockWorkerEffectsUnknown
	dependencyWaitgroupLockWorkerOrderNotMatched
	dependencyReasonCount
)

var dependencyReasonCodes = [...]string{
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
	dependencyLockJoinPathsInfeasible:             "lock-join-paths-infeasible",
	dependencyLockJoinPathsUnknown:                "lock-join-path-feasibility-unknown",
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
	dependencyWaitgroupLockPathsInfeasible:        "waitgroup-lock-paths-infeasible",
	dependencyWaitgroupLockPathsUnknown:           "waitgroup-lock-path-feasibility-unknown",
	dependencyWaitgroupLockShapeNotMatched:        "waitgroup-lock-shape-not-matched",
	dependencyWaitgroupLockSourceUnknown:          "waitgroup-lock-source-unknown",
	dependencyWaitgroupLockWorkerEffectsUnknown:   "waitgroup-lock-worker-effects-unknown",
	dependencyWaitgroupLockWorkerOrderNotMatched:  "waitgroup-lock-worker-order-not-matched",
}

func (reason dependencyReason) String() string {
	if int(reason) >= len(dependencyReasonCodes) {
		return "invalid-lockorder-reason"
	}
	return dependencyReasonCodes[reason]
}
