package channelsafety

import "testing"

func TestChannelCycleReasonCodes(t *testing.T) {
	want := map[channelCycleReason]string{
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
	if len(want) != int(channelCycleReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range channelCycleReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []channelCycleReason{channelCycleReasonCount, 255} {
		if reason.String() != "invalid-channelsafety-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
