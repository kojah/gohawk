package channelsafety

// channelCycleReason classifies this check's evidence independently of upstream model failures.
type channelCycleReason uint8

const (
	channelCycleNone channelCycleReason = iota
	channelCycleAlternativeSiteUnknown
	channelCycleAlternativeSitesDiffer
	channelCycleAlternativeUnproven
	channelCycleAlternativesUnknown
	channelCycleCandidate
	channelCycleDependenciesUnproven
	channelCycleFreshChannelsUnknown
	channelCycleLaunchUnknown
	channelCycleOtherParticipant
	channelCycleParentSequenceUnknown
	channelCyclePathsInfeasible
	channelCyclePathsUnknown
	channelCyclePartnerUnknown
	channelCycleProven
	channelCycleResourceOrOrderUnknown
	channelCycleSourceUnknown
	channelCycleSpawnOrderUnknown
	channelCycleSummaryUnavailable
	channelCycleWorkerSequenceUnknown
	channelCycleWorkerUnknown
	channelCycleReasonCount
)

var channelCycleReasonCodes = [...]string{
	channelCycleNone:                   "",
	channelCycleAlternativeSiteUnknown: "channel-cycle-alternative-site-unknown",
	channelCycleAlternativeSitesDiffer: "channel-cycle-alternative-sites-differ",
	channelCycleAlternativeUnproven:    "channel-cycle-alternative-unproven",
	channelCycleAlternativesUnknown:    "channel-cycle-alternatives-unknown",
	channelCycleCandidate:              "channel-cycle-candidate",
	channelCycleDependenciesUnproven:   "channel-cycle-dependencies-unproven",
	channelCycleFreshChannelsUnknown:   "channel-cycle-fresh-channels-unknown",
	channelCycleLaunchUnknown:          "channel-cycle-launch-unknown",
	channelCycleOtherParticipant:       "channel-cycle-other-participant",
	channelCycleParentSequenceUnknown:  "channel-cycle-parent-sequence-unknown",
	channelCyclePathsInfeasible:        "channel-cycle-paths-infeasible",
	channelCyclePathsUnknown:           "channel-cycle-path-feasibility-unknown",
	channelCyclePartnerUnknown:         "channel-cycle-partner-unknown",
	channelCycleProven:                 "channel-cycle-proven",
	channelCycleResourceOrOrderUnknown: "channel-cycle-resource-or-order-unknown",
	channelCycleSourceUnknown:          "channel-cycle-source-unknown",
	channelCycleSpawnOrderUnknown:      "channel-cycle-spawn-order-unknown",
	channelCycleSummaryUnavailable:     "channel-cycle-summary-unavailable",
	channelCycleWorkerSequenceUnknown:  "channel-cycle-worker-sequence-unknown",
	channelCycleWorkerUnknown:          "channel-cycle-worker-unknown",
}

func (reason channelCycleReason) String() string {
	if int(reason) >= len(channelCycleReasonCodes) {
		return "invalid-channelsafety-reason"
	}
	return channelCycleReasonCodes[reason]
}
