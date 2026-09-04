package analyzers

import (
	"github.com/kojah/gohawk/internal/analyzers/contracts/apishape"
	"github.com/kojah/gohawk/internal/analyzers/contracts/closedomain"
	"github.com/kojah/gohawk/internal/analyzers/contracts/contextpolicy"
	"github.com/kojah/gohawk/internal/analyzers/contracts/wirepolicy"
	"github.com/kojah/gohawk/internal/analyzers/ownership/borrowedstorage"
	"github.com/kojah/gohawk/internal/analyzers/ownership/cancellationownership"
	"github.com/kojah/gohawk/internal/analyzers/ownership/channelcapacity"
	"github.com/kojah/gohawk/internal/analyzers/ownership/channelownership"
	"github.com/kojah/gohawk/internal/analyzers/ownership/channelsafety"
	"github.com/kojah/gohawk/internal/analyzers/ownership/deferinloop"
	"github.com/kojah/gohawk/internal/analyzers/ownership/exitpolicy"
	"github.com/kojah/gohawk/internal/analyzers/ownership/goroutineownership"
	"github.com/kojah/gohawk/internal/analyzers/ownership/processownership"
	"github.com/kojah/gohawk/internal/analyzers/ownership/producerlifecycle"
	"github.com/kojah/gohawk/internal/analyzers/ownership/resourcelifetime"
	"github.com/kojah/gohawk/internal/analyzers/reliability/concurrentcapture"
	"github.com/kojah/gohawk/internal/analyzers/reliability/determinism"
	"github.com/kojah/gohawk/internal/analyzers/reliability/errorclassification"
	"github.com/kojah/gohawk/internal/analyzers/reliability/errorownership"
	"github.com/kojah/gohawk/internal/analyzers/reliability/evalorder"
	"github.com/kojah/gohawk/internal/analyzers/reliability/globalstate"
	"github.com/kojah/gohawk/internal/analyzers/reliability/inlineerror"
	"github.com/kojah/gohawk/internal/analyzers/reliability/lockorder"
	"github.com/kojah/gohawk/internal/analyzers/reliability/oncepolicy"
	"github.com/kojah/gohawk/internal/analyzers/reliability/syncmapatomicity"
	"github.com/kojah/gohawk/internal/analyzers/reliability/taintpolicy"
	"github.com/kojah/gohawk/internal/analyzers/testing/testlifecycle"
	"github.com/kojah/gohawk/internal/analyzers/testing/testpolicy"
	"github.com/kojah/gohawk/internal/catalog"
	"github.com/kojah/gohawk/internal/check"
)

func contractSpecs() []catalog.AnalyzerSpec {
	return []catalog.AnalyzerSpec{
		{
			Analyzer: apishape.Analyzer(),
			Checks: []catalog.CheckInfo{
				{
					ID: check.APIParameterCount, Doc: "Reports unconstrained exported APIs with more than the configured maximum number of parameters.",
					Kind: catalog.KindPolicy, Tier: catalog.TierExtended, Delisted: true,
				},
				{
					ID: check.APIMixedReceivers, Doc: "Reports types that mix pointer and value receiver methods.",
					Kind: catalog.KindPolicy, Tier: catalog.TierExtended, Delisted: true,
				},
				{
					ID: check.APIAdjacentSameType, Doc: "Reports long adjacent runs of parameters in unconstrained signatures that share one type.",
					Kind: catalog.KindPolicy, Tier: catalog.TierExtended, Delisted: true,
				},
				{
					ID: check.APIAdjacentOptional, Doc: "Reports adjacent optional scalar parameters in unconstrained signatures that are easy to swap.",
					Kind: catalog.KindPolicy, Tier: catalog.TierExtended, Delisted: true,
				},
			},
		},
		{
			Analyzer: contextpolicy.Analyzer(),
			Checks: []catalog.CheckInfo{
				{
					ID: check.ContextFirst,
					Doc: "Reports misplaced context.Context parameters while allowing additional contexts " +
						"after a leading context and one context after a testing handle.",
					Kind: catalog.KindPolicy, Tier: catalog.TierCore, Delisted: true,
				},
				{ID: check.ContextStorage, Doc: "Reports context.Context values stored in structs.", Kind: catalog.KindPolicy, Tier: catalog.TierCore, Delisted: true},
				{ID: check.ContextNilArgument, Doc: "Reports definitely nil context.Context arguments.", Kind: catalog.KindDefect, Tier: catalog.TierCore, Delisted: true},
			},
		},
		{Analyzer: closedomain.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ClosedStringDomain, Doc: "Reports exported string fields used as small closed sets of values.",
				Kind: catalog.KindPolicy, Tier: catalog.TierExtended, Delisted: true,
			},
		}},
		{Analyzer: wirepolicy.Analyzer(), SuggestedFix: true, Checks: []catalog.CheckInfo{
			{
				ID: check.WireKeyedLiteral, Doc: "Reports positional composite literals for persisted or wire structs.",
				Kind: catalog.KindPolicy, Tier: catalog.TierExtended, Delisted: true,
			},
			{
				ID: check.WireSerializationTag, Doc: "Reports exported wire fields without explicit JSON or TOML tags.",
				Kind: catalog.KindPolicy, Tier: catalog.TierExtended, Delisted: true,
			},
		}},
	}
}

func ownershipSpecs() []catalog.AnalyzerSpec {
	return []catalog.AnalyzerSpec{
		{Analyzer: borrowedstorage.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID:   check.BorrowedStorageOwner,
				Doc:  "Reports borrowed bytes.Buffer storage transferred to a second escaping owner without a copy.",
				Kind: catalog.KindHazard, Tier: catalog.TierExperimental,
			},
		}},
		{Analyzer: cancellationownership.Analyzer(), SuggestedFix: true, Checks: []catalog.CheckInfo{
			{
				ID: check.CancellationRelease, Doc: "Reports derived cancel functions proved lost on a feasible normal return path.",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: channelcapacity.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ChannelCapacityRationale, Doc: "Reports large constant channel capacities in production files without a nearby bounded rationale.",
				Kind: catalog.KindPolicy, Tier: catalog.TierExtended, Delisted: true,
			},
		}},
		{Analyzer: channelownership.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ChannelCallerClose, Doc: "Reports callees that close a channel an exact caller continues to use.",
				Kind: catalog.KindPolicy, Tier: catalog.TierExtended, Delisted: true,
			},
		}},
		{Analyzer: channelsafety.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ChannelSendAfterClose, Doc: "Reports sends reachable after a channel has been closed.",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: deferinloop.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.DeferCleanupInLoop, Doc: "Reports cleanup defers whose lifetime extends across loop iterations.",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: exitpolicy.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ExitSkipsDefer, Doc: "Reports immediate process termination that bypasses an earlier defer.",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: goroutineownership.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.GoroutineJoin, Doc: "Reports goroutines with a recognizable join or lifecycle mechanism that is not honored on every return path.",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
			{
				ID: check.GoroutineDetached, Doc: "Reports goroutines without a recognizable join handle or lifecycle owner.",
				Kind: catalog.KindHazard, Tier: catalog.TierExperimental,
			},
		}},
		{Analyzer: producerlifecycle.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ProducerLifecycleSend, Doc: "Reports producer goroutines that can block after their receiver stops waiting.",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: processownership.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ProcessWait, Doc: "Reports successfully started commands that are neither waited on nor transferred.",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
			{
				ID:   check.ProcessDetached,
				Doc:  "Reports started commands whose handle is never used again, the fire-and-forget launch.",
				Kind: catalog.KindPolicy,
				Tier: catalog.TierExperimental, Delisted: true,
			},
		}},
		{Analyzer: resourcelifetime.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ResourceRelease, Doc: "Reports owned resources that are not released on every return path.",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
			{
				ID:   check.ResourceUseAfterRelease,
				Doc:  "Reports an invalidating operation on a resource that a direct release dominates.",
				Kind: catalog.KindHazard,
				Tier: catalog.TierExperimental,
			},
		}},
	}
}

func reliabilitySpecs() []catalog.AnalyzerSpec {
	return []catalog.AnalyzerSpec{
		{Analyzer: concurrentcapture.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ConcurrentCapture, Doc: "Reports repeatedly launched goroutines that mutate the same captured local.",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: determinism.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.DeterministicMapOutput, Doc: "Reports map iteration that reaches ordered output without explicit sorting.",
				Kind: catalog.KindHazard, Tier: catalog.TierExtended,
			},
		}},
		{Analyzer: errorownership.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ErrorLogAndReturn, Doc: "Reports functions that both log and return the same error.",
				Kind: catalog.KindPolicy, Tier: catalog.TierExtended, Delisted: true,
			},
		}},
		{Analyzer: errorclassification.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ErrorTextClassification, Doc: "Reports production code that classifies errors by matching their text.",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: inlineerror.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.ErrorMismatchedInline, Doc: "Reports inline error declarations whose condition checks a different error.",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: evalorder.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.EvaluationOrder, Doc: "Reports expressions whose later operand mutates a value read by an earlier operand.",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: globalstate.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.MutableGlobalState, Doc: "Reports mutable package-level state without an explicit owner.",
				Kind: catalog.KindPolicy, Tier: catalog.TierExtended, Delisted: true,
			},
		}},
		{Analyzer: lockorder.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.LockMissingRelease, Doc: "Reports return paths that leave an owned lock held.", Kind: catalog.KindDefect, Tier: catalog.TierCore},
			{
				ID: check.LockRecursiveAcquire, Doc: "Reports attempts to acquire a lock that is already held.",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
			{
				ID: check.LockContradictoryOrder, Doc: "Reports two locks acquired in opposite orders in different places.",
				Kind: catalog.KindHazard, Tier: catalog.TierExtended,
			},
			{
				ID: check.LockReadLockWrite, Doc: "Reports writes to an object while only its read lock is held.",
				Kind: catalog.KindHazard, Tier: catalog.TierExperimental,
			},
			{
				ID: check.LockMismatchedRelease, Doc: "Reports a lock released with the wrong method for how it was acquired.",
				Kind: catalog.KindDefect, Tier: catalog.TierExperimental,
			},
			{
				ID: check.LockDiscardedTryLock, Doc: "Reports a TryLock whose result is discarded, leaving the lock possibly unheld.",
				Kind: catalog.KindDefect, Tier: catalog.TierExperimental,
			},
		}},
		{Analyzer: oncepolicy.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.OnceDiscardedWrapper, Doc: "Reports sync.Once function wrappers that are called and immediately discarded.",
				Kind: catalog.KindDefect, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: syncmapatomicity.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.SyncMapNonAtomicClaim, Doc: "Reports separate sync.Map Load and Delete operations used to claim one value.",
				Kind: catalog.KindHazard, Tier: catalog.TierCore,
			},
		}},
		{Analyzer: taintpolicy.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.TaintUntrustedSink, Doc: "Reports untrusted input that reaches a configured sensitive sink without validation.",
				Kind: catalog.KindHazard, Tier: catalog.TierExperimental, Delisted: true,
			},
		}},
	}
}

func testingSpecs() []catalog.AnalyzerSpec {
	return []catalog.AnalyzerSpec{
		{Analyzer: testlifecycle.Analyzer(goroutineownership.GoroutineOwnershipMayBeHandledInTest), Checks: []catalog.CheckInfo{
			{
				ID: check.TestLifecycleContext, Doc: "Reports detached test-owned goroutines rooted in a never-cancelled context.",
				Kind: catalog.KindHazard, Tier: catalog.TierExtended, Delisted: true,
			},
		}},
		{
			Analyzer: testpolicy.Analyzer(), SuggestedFix: true,
			Checks: []catalog.CheckInfo{{
				ID: check.TestHelperMarker, Doc: "Reports test helpers that do not call Helper on every return path.",
				Kind: catalog.KindPolicy, Tier: catalog.TierExtended, Delisted: true,
			}},
		},
	}
}
