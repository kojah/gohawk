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
				Help: "close a channel only from its sender, after the last send",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: concurrentcapture.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ConcurrentCapture, Doc: "Reports repeatedly launched goroutines that mutate the same captured local.",
				Help: "give each goroutine its own copy of the value, or collect results through a channel or under a mutex",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: goroutineownership.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.GoroutineJoin, Doc: "Reports goroutines with a recognizable join or lifecycle mechanism that is not honored on every return path.",
				Help: "wait for the goroutine on every return path, for example with a sync.WaitGroup or by receiving from its done channel",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: lockorder.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.LockMissingRelease, Doc: "Reports return paths that leave an owned lock held.",
				Help: "unlock on every return path, usually with `defer mu.Unlock()` right after locking",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
			{
				ID: check.LockRecursiveAcquire, Doc: "Reports attempts to acquire a lock that is already held.",
				Help: "release the lock before calling code that locks it again, or give callers that hold it an unlocked helper",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
			{
				ID: check.LockContradictoryOrder, Doc: "Reports bounded cycles in mutex acquisition order, with acquisition and helper-call evidence.",
				Help: "choose one order for acquiring these locks and use it everywhere",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
			{
				ID: check.LockReadLockWrite, Doc: "Reports writes to an object while only its read lock is held.",
				Help: "take the write lock with Lock, not RLock, before modifying the object",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: producerlifecycle.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ProducerLifecycleSend, Doc: "Reports producer goroutines that can block after their receiver stops waiting.",
				Help: "let the producer stop when the receiver does, for example with a context or done channel, or buffer every send",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
			{
				ID: check.ProducerLifecycleStoppedLoop, Doc: "Reports sends that can block forever after the service loop receiving them returns.",
				Help: "stop the senders before the service loop returns, or have them also select on a done channel",
				Kind: catalog.KindHazard, Tier: catalog.TierExperimental,
			},
			{
				ID:   check.ProducerLifecycleUnclosed,
				Doc:  "Reports range loops that wait forever when their producer returns an error without closing the channel.",
				Help: "close the channel on every return of the producer, including error returns, usually with `defer close(ch)`",
				Kind: catalog.KindHazard, Tier: catalog.TierExperimental,
			},
			{
				ID:   check.ProducerLifecycleUnreceivedReturn,
				Doc:  "Reports goroutines left blocked on a send when the function that launched them returns without receiving.",
				Help: "receive the result on every return path, or give the channel a buffer of one so the send completes without a receiver",
				Kind: catalog.KindDefect, Tier: catalog.TierExperimental,
			},
			{
				ID:   check.ProducerLifecycleUnsignalledReceiver,
				Doc:  "Reports goroutines left waiting on a channel that the function launching them returns without sending on or closing.",
				Help: "close the channel on every return path, usually with `defer close(ch)` right after making it",
				Kind: catalog.KindDefect, Tier: catalog.TierExperimental,
			},
		}},
	}
}

func resourcesSpecs() []catalog.AnalyzerSpec {
	return []catalog.AnalyzerSpec{
		{Analyzer: cancellationownership.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.CancellationRelease, Doc: "Reports derived cancel functions proved lost on a feasible normal return path.",
				Help: "call the cancel function on every return path, usually with `defer cancel()` right after creating it",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: deferinloop.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.DeferCleanupInLoop, Doc: "Reports cleanup defers whose lifetime extends across loop iterations.",
				Help: "move the loop body into a function so its deferred cleanup runs at the end of each iteration",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: processownership.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ProcessWait, Doc: "Reports successfully started commands that are neither waited on nor transferred.",
				Help: "call Wait on every return path after Start succeeds, or hand the command to code that waits for it",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: resourcelifetime.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ResourceRelease, Doc: "Reports owned resources that are not released on every return path.",
				Help: "release it on every return path, usually with `defer` right after the error check",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
			{
				ID:   check.ResourceUseAfterRelease,
				Doc:  "Reports an invalidating operation on the same resource after a dominating release, with no intervening unknown effects.",
				Help: "finish using the resource before releasing it, or acquire a new one",
				Kind: catalog.KindHazard,
				Tier: catalog.TierCore,
			},
		}},
	}
}
