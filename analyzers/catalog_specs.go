package analyzers

import (
	"github.com/kojah/gohawk/internal/analyzers/concurrency/channelsafety"
	"github.com/kojah/gohawk/internal/analyzers/concurrency/concurrentcapture"
	"github.com/kojah/gohawk/internal/analyzers/concurrency/goroutineownership"
	"github.com/kojah/gohawk/internal/analyzers/concurrency/lockorder"
	"github.com/kojah/gohawk/internal/analyzers/concurrency/producerlifecycle"
	"github.com/kojah/gohawk/internal/analyzers/resources/cancellationownership"
	"github.com/kojah/gohawk/internal/analyzers/resources/deferinloop"
	"github.com/kojah/gohawk/internal/analyzers/resources/processownership"
	"github.com/kojah/gohawk/internal/analyzers/resources/resourcelifetime"
	"github.com/kojah/gohawk/internal/catalog"
	"github.com/kojah/gohawk/internal/check"
)

func concurrencySpecs() []catalog.AnalyzerSpec {
	return []catalog.AnalyzerSpec{
		{Analyzer: channelsafety.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ChannelSendAfterClose, Doc: "Reports sends reachable after a channel has been closed.",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
			{
				ID: check.ChannelDependencyCycle, Doc: "Reports two exact unbuffered channel operations that mutually wait on later matching operations.",
				Kind: catalog.KindDefect, Tier: catalog.TierExperimental,
			},
		}},
		{Analyzer: concurrentcapture.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ConcurrentCapture, Doc: "Reports repeatedly launched goroutines that mutate the same captured local.",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: goroutineownership.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.GoroutineJoin, Doc: "Reports goroutines with a recognizable join or lifecycle mechanism that is not honored on every return path.",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: lockorder.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.LockMissingRelease, Doc: "Reports return paths that leave an owned lock held.", Kind: catalog.KindDefect, Tier: catalog.TierCore},
			{
				ID: check.LockRecursiveAcquire, Doc: "Reports attempts to acquire a lock that is already held.",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
			{
				ID: check.LockContradictoryOrder, Doc: "Reports bounded cycles in mutex acquisition order, with acquisition and helper-call evidence.",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
			{
				ID: check.LockAndJoin, Doc: "Reports a wait while holding a mutex that every possible completion worker must acquire.",
				Kind: catalog.KindDefect, Tier: catalog.TierExperimental,
			},
			{
				ID: check.LockChannelCycle, Doc: "Reports a receive on a fresh unbuffered channel whose possible senders first need the receiver's held mutex.",
				Kind: catalog.KindDefect, Tier: catalog.TierExperimental,
			},
			{
				ID: check.LockWaitGroupCycle, Doc: "Reports a WaitGroup wait whose counted workers all need the caller's held mutex before Done.",
				Kind: catalog.KindDefect, Tier: catalog.TierExperimental,
			},
			{
				ID: check.LockReadLockWrite, Doc: "Reports writes to an object while only its read lock is held.",
				Kind: catalog.KindHazard, Tier: catalog.TierExperimental,
			},
			{
				ID: check.LockMismatchedRelease, Doc: "Reports a lock released with the wrong method for how it was acquired.",
				Kind: catalog.KindDefect, Tier: catalog.TierExperimental,
			},
		}},
		{Analyzer: producerlifecycle.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ProducerLifecycleSend, Doc: "Reports producer goroutines that can block after their receiver stops waiting.",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
			{
				ID: check.ProducerLifecycleStoppedLoop, Doc: "Reports sends that can block forever after the service loop receiving them returns.",
				Kind: catalog.KindHazard, Tier: catalog.TierExperimental,
			},
			{
				ID:   check.ProducerLifecycleUnclosed,
				Doc:  "Reports range loops that wait forever when their producer returns an error without closing the channel.",
				Kind: catalog.KindHazard, Tier: catalog.TierExperimental,
			},
		}},
	}
}

func resourcesSpecs() []catalog.AnalyzerSpec {
	return []catalog.AnalyzerSpec{
		{Analyzer: cancellationownership.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.CancellationRelease, Doc: "Reports derived cancel functions proved lost on a feasible normal return path.",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: deferinloop.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.DeferCleanupInLoop, Doc: "Reports cleanup defers whose lifetime extends across loop iterations.",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: processownership.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ProcessWait, Doc: "Reports successfully started commands that are neither waited on nor transferred.",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: resourcelifetime.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ResourceRelease, Doc: "Reports owned resources that are not released on every return path.",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
			{
				ID:   check.ResourceUseAfterRelease,
				Doc:  "Reports an invalidating operation on the same resource after a dominating release, with no intervening unknown effects.",
				Kind: catalog.KindHazard,
				Tier: catalog.TierCore,
			},
		}},
	}
}
