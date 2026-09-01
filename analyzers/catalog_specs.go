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
			Analyzer: apishape.Analyzer(), OptIn: true,
			Checks: []catalog.CheckInfo{
				{
					ID: check.APIParameterCount, Doc: "Reports unconstrained exported APIs with more than the configured maximum number of parameters.",
					Kind: catalog.KindPolicy,
				},
				{ID: check.APIMixedReceivers, Doc: "Reports types that mix pointer and value receiver methods.", Kind: catalog.KindPolicy},
				{
					ID: check.APIAdjacentSameType, Doc: "Reports long adjacent runs of parameters in unconstrained signatures that share one type.",
					Kind: catalog.KindPolicy,
				},
				{
					ID: check.APIAdjacentOptional, Doc: "Reports adjacent optional scalar parameters in unconstrained signatures that are easy to swap.",
					Kind: catalog.KindPolicy,
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
					Kind: catalog.KindPolicy,
				},
				{ID: check.ContextStorage, Doc: "Reports context.Context values stored in structs.", Kind: catalog.KindPolicy},
				{ID: check.ContextNilArgument, Doc: "Reports definitely nil context.Context arguments.", Kind: catalog.KindDefect},
			},
		},
		{Analyzer: closedomain.Analyzer(), OptIn: true, Checks: []catalog.CheckInfo{
			{ID: check.ClosedStringDomain, Doc: "Reports exported string fields used as small closed sets of values.", Kind: catalog.KindPolicy},
		}},
		{Analyzer: wirepolicy.Analyzer(), OptIn: true, SuggestedFix: true, Checks: []catalog.CheckInfo{
			{ID: check.WireKeyedLiteral, Doc: "Reports positional composite literals for persisted or wire structs.", Kind: catalog.KindPolicy},
			{ID: check.WireSerializationTag, Doc: "Reports exported wire fields without explicit JSON or TOML tags.", Kind: catalog.KindPolicy},
		}},
	}
}

func ownershipSpecs() []catalog.AnalyzerSpec {
	return []catalog.AnalyzerSpec{
		{Analyzer: borrowedstorage.Analyzer(), OptIn: true, Checks: []catalog.CheckInfo{
			{
				ID:   check.BorrowedStorageOwner,
				Doc:  "Reports borrowed bytes.Buffer storage transferred to a second escaping owner without a copy.",
				Kind: catalog.KindHazard,
			},
		}},
		{Analyzer: cancellationownership.Analyzer(), SuggestedFix: true, Checks: []catalog.CheckInfo{
			{
				ID: check.CancellationRelease, Doc: "Reports derived cancel functions that are neither called nor transferred on every return path.",
				Kind: catalog.KindDefect,
			},
		}},
		{Analyzer: channelcapacity.Analyzer(), OptIn: true, Checks: []catalog.CheckInfo{
			{
				ID: check.ChannelCapacityRationale, Doc: "Reports large constant channel capacities in production files without a nearby bounded rationale.",
				Kind: catalog.KindPolicy,
			},
		}},
		{Analyzer: channelownership.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.ChannelCallerClose, Doc: "Reports functions that close channels received from their callers.", Kind: catalog.KindPolicy},
		}},
		{Analyzer: channelsafety.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.ChannelSendAfterClose, Doc: "Reports sends reachable after a channel has been closed.", Kind: catalog.KindDefect},
		}},
		{Analyzer: deferinloop.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.DeferCleanupInLoop, Doc: "Reports cleanup defers whose lifetime extends across loop iterations.", Kind: catalog.KindHazard},
		}},
		{Analyzer: exitpolicy.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.ExitSkipsDefer, Doc: "Reports immediate process termination that bypasses an earlier defer.", Kind: catalog.KindDefect},
		}},
		{Analyzer: goroutineownership.Analyzer(), Checks: []catalog.CheckInfo{
			{
				ID: check.GoroutineJoin, Doc: "Reports goroutines with a recognizable join or lifecycle mechanism that is not honored on every return path.",
				Kind: catalog.KindHazard,
			},
			{
				ID: check.GoroutineDetached, Doc: "Reports goroutines without a recognizable join handle or lifecycle owner.",
				Kind: catalog.KindHazard, OptIn: true,
			},
		}},
		{Analyzer: producerlifecycle.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.ProducerLifecycleSend, Doc: "Reports producer goroutines that can block after their receiver stops waiting.", Kind: catalog.KindHazard},
		}},
		{Analyzer: processownership.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.ProcessWait, Doc: "Reports successfully started commands that are neither waited on nor transferred.", Kind: catalog.KindDefect},
		}},
		{Analyzer: resourcelifetime.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.ResourceRelease, Doc: "Reports owned resources that are not released on every return path.", Kind: catalog.KindDefect},
		}},
	}
}

func reliabilitySpecs() []catalog.AnalyzerSpec {
	return []catalog.AnalyzerSpec{
		{Analyzer: concurrentcapture.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.ConcurrentCapture, Doc: "Reports repeatedly launched goroutines that mutate the same captured local.", Kind: catalog.KindHazard},
		}},
		{Analyzer: determinism.Analyzer(), OptIn: true, Checks: []catalog.CheckInfo{
			{ID: check.DeterministicMapOutput, Doc: "Reports map iteration that reaches ordered output without explicit sorting.", Kind: catalog.KindHazard},
		}},
		{Analyzer: errorownership.Analyzer(), OptIn: true, Checks: []catalog.CheckInfo{
			{ID: check.ErrorLogAndReturn, Doc: "Reports functions that both log and return the same error.", Kind: catalog.KindPolicy},
		}},
		{Analyzer: errorclassification.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.ErrorTextClassification, Doc: "Reports production code that classifies errors by matching their text.", Kind: catalog.KindHazard},
		}},
		{Analyzer: inlineerror.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.ErrorMismatchedInline, Doc: "Reports inline error declarations whose condition checks a different error.", Kind: catalog.KindDefect},
		}},
		{Analyzer: evalorder.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.EvaluationOrder, Doc: "Reports expressions whose later operand mutates a value read by an earlier operand.", Kind: catalog.KindHazard},
		}},
		{Analyzer: globalstate.Analyzer(), OptIn: true, Checks: []catalog.CheckInfo{
			{ID: check.MutableGlobalState, Doc: "Reports mutable package-level state without an explicit owner.", Kind: catalog.KindPolicy},
		}},
		{Analyzer: lockorder.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.LockMissingRelease, Doc: "Reports return paths that leave an owned lock held.", Kind: catalog.KindDefect},
			{ID: check.LockRecursiveAcquire, Doc: "Reports attempts to acquire a lock that is already held.", Kind: catalog.KindDefect},
			{ID: check.LockContradictoryOrder, Doc: "Reports inconsistent acquisition order for the same pair of locks.", Kind: catalog.KindHazard},
		}},
		{Analyzer: oncepolicy.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.OnceDiscardedWrapper, Doc: "Reports sync.Once function wrappers that are called and immediately discarded.", Kind: catalog.KindDefect},
		}},
		{Analyzer: syncmapatomicity.Analyzer(), Checks: []catalog.CheckInfo{
			{ID: check.SyncMapNonAtomicClaim, Doc: "Reports separate sync.Map Load and Delete operations used to claim one value.", Kind: catalog.KindHazard},
		}},
		{Analyzer: taintpolicy.Analyzer(), OptIn: true, Checks: []catalog.CheckInfo{
			{
				ID: check.TaintUntrustedSink, Doc: "Reports untrusted input that reaches a configured sensitive sink without validation.",
				Kind: catalog.KindHazard,
			},
		}},
	}
}

func testingSpecs() []catalog.AnalyzerSpec {
	return []catalog.AnalyzerSpec{
		{Analyzer: testlifecycle.Analyzer(), OptIn: true, Checks: []catalog.CheckInfo{
			{ID: check.TestLifecycleContext, Doc: "Reports detached test-owned goroutines rooted in a never-cancelled context.", Kind: catalog.KindHazard},
		}},
		{
			Analyzer: testpolicy.Analyzer(), OptIn: true, SuggestedFix: true,
			Checks: []catalog.CheckInfo{{
				ID: check.TestHelperMarker, Doc: "Reports test helpers that do not call Helper on every return path.", Kind: catalog.KindPolicy,
			}},
		},
	}
}
