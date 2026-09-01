package analyzers

import (
	"github.com/kojah/gohawk/internal/analyzerbase"
	"github.com/kojah/gohawk/internal/analyzers/contracts/apishape"
	"github.com/kojah/gohawk/internal/analyzers/contracts/closedomain"
	"github.com/kojah/gohawk/internal/analyzers/contracts/contextpolicy"
	"github.com/kojah/gohawk/internal/analyzers/contracts/wirepolicy"
	"github.com/kojah/gohawk/internal/analyzers/ownership/cancellationownership"
	"github.com/kojah/gohawk/internal/analyzers/ownership/channelpolicy"
	"github.com/kojah/gohawk/internal/analyzers/ownership/deferinloop"
	"github.com/kojah/gohawk/internal/analyzers/ownership/exitpolicy"
	"github.com/kojah/gohawk/internal/analyzers/ownership/goroutineownership"
	"github.com/kojah/gohawk/internal/analyzers/ownership/processownership"
	"github.com/kojah/gohawk/internal/analyzers/ownership/resourcelifetime"
	"github.com/kojah/gohawk/internal/analyzers/reliability/concurrentcapture"
	"github.com/kojah/gohawk/internal/analyzers/reliability/determinism"
	"github.com/kojah/gohawk/internal/analyzers/reliability/errorownership"
	"github.com/kojah/gohawk/internal/analyzers/reliability/evalorder"
	"github.com/kojah/gohawk/internal/analyzers/reliability/globalstate"
	"github.com/kojah/gohawk/internal/analyzers/reliability/lockorder"
	"github.com/kojah/gohawk/internal/analyzers/reliability/oncepolicy"
	"github.com/kojah/gohawk/internal/analyzers/reliability/syncmapatomicity"
	"github.com/kojah/gohawk/internal/analyzers/reliability/taintpolicy"
	"github.com/kojah/gohawk/internal/analyzers/testing/testpolicy"
)

func contractSpecs() []analyzerbase.AnalyzerSpec {
	return []analyzerbase.AnalyzerSpec{
		{
			Analyzer: apishape.Analyzer(), OptIn: true,
			Checks: []analyzerbase.CheckInfo{
				{ID: analyzerbase.CheckAPIParameterCount, Doc: "Reports unconstrained exported APIs with more than the configured maximum number of parameters."},
				{ID: analyzerbase.CheckAPIMixedReceivers, Doc: "Reports types that mix pointer and value receiver methods."},
				{ID: analyzerbase.CheckAPIAdjacentSameType, Doc: "Reports long adjacent runs of parameters in unconstrained signatures that share one type."},
				{ID: analyzerbase.CheckAPIAdjacentOptional, Doc: "Reports adjacent optional scalar parameters in unconstrained signatures that are easy to swap."},
			},
		},
		{
			Analyzer: contextpolicy.Analyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: analyzerbase.CheckContextFirst, Doc: "Reports misplaced context.Context parameters while allowing additional contexts after a leading context and one context after a testing handle."},
				{ID: analyzerbase.CheckContextStorage, Doc: "Reports context.Context values stored in structs."},
				{ID: analyzerbase.CheckContextTestOwnership, Doc: "Reports detached test-owned goroutines rooted in a never-cancelled context.", OptIn: true},
				{ID: analyzerbase.CheckContextNilArgument, Doc: "Reports definitely nil context.Context arguments."},
			},
		},
		{Analyzer: closedomain.Analyzer(), OptIn: true, Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckClosedStringDomain, Doc: "Reports exported string fields used as small closed sets of values."},
		}},
		{Analyzer: wirepolicy.Analyzer(), OptIn: true, SuggestedFix: true, Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckWireKeyedLiteral, Doc: "Reports positional composite literals for persisted or wire structs."},
			{ID: analyzerbase.CheckWireSerializationTag, Doc: "Reports exported wire fields without explicit JSON or TOML tags."},
		}},
	}
}

func ownershipSpecs() []analyzerbase.AnalyzerSpec {
	return []analyzerbase.AnalyzerSpec{
		{Analyzer: cancellationownership.Analyzer(), SuggestedFix: true, Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckCancellationRelease, Doc: "Reports derived cancel functions that are neither called nor transferred on every return path."},
		}},
		{Analyzer: channelpolicy.Analyzer(), Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckChannelCapacityRationale, Doc: "Reports large constant channel capacities in production files without a nearby bounded rationale.", OptIn: true},
			{ID: analyzerbase.CheckChannelCallerClose, Doc: "Reports functions that close channels received from their callers."},
			{ID: analyzerbase.CheckChannelSendAfterClose, Doc: "Reports sends reachable after a channel has been closed."},
		}},
		{Analyzer: deferinloop.Analyzer(), Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckDeferCleanupInLoop, Doc: "Reports cleanup defers whose lifetime extends across loop iterations."},
		}},
		{Analyzer: exitpolicy.Analyzer(), Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckExitSkipsDefer, Doc: "Reports immediate process termination that bypasses an earlier defer."},
		}},
		{Analyzer: goroutineownership.Analyzer(), Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckGoroutineJoin, Doc: "Reports goroutines with a recognizable join or lifecycle mechanism that is not honored on every return path."},
			{ID: analyzerbase.CheckGoroutineDetached, Doc: "Reports goroutines without a recognizable join handle or lifecycle owner.", OptIn: true},
			{ID: analyzerbase.CheckGoroutineProducerSend, Doc: "Reports producer goroutines that can block after their receiver stops waiting."},
		}},
		{Analyzer: processownership.Analyzer(), Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckProcessWait, Doc: "Reports successfully started commands that are neither waited on nor transferred."},
		}},
		{Analyzer: resourcelifetime.Analyzer(), Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckResourceRelease, Doc: "Reports owned resources that are not released on every return path."},
		}},
	}
}

func reliabilitySpecs() []analyzerbase.AnalyzerSpec {
	return []analyzerbase.AnalyzerSpec{
		{Analyzer: concurrentcapture.Analyzer(), Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckConcurrentCapture, Doc: "Reports repeatedly launched goroutines that mutate the same captured local."},
		}},
		{Analyzer: determinism.Analyzer(), OptIn: true, Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckDeterministicMapOutput, Doc: "Reports map iteration that reaches ordered output without explicit sorting."},
		}},
		{Analyzer: errorownership.Analyzer(), Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckErrorLogAndReturn, Doc: "Reports functions that both log and return the same error.", OptIn: true},
			{ID: analyzerbase.CheckErrorTextClassification, Doc: "Reports production code that classifies errors by matching their text."},
			{ID: analyzerbase.CheckErrorMismatchedInline, Doc: "Reports inline error declarations whose condition checks a different error."},
		}},
		{Analyzer: evalorder.Analyzer(), Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckEvaluationOrder, Doc: "Reports expressions whose later operand mutates a value read by an earlier operand."},
		}},
		{Analyzer: globalstate.Analyzer(), OptIn: true, Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckMutableGlobalState, Doc: "Reports mutable package-level state without an explicit owner."},
		}},
		{Analyzer: lockorder.Analyzer(), Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckLockMissingRelease, Doc: "Reports return paths that leave an owned lock held."},
			{ID: analyzerbase.CheckLockRecursiveAcquire, Doc: "Reports attempts to acquire a lock that is already held."},
			{ID: analyzerbase.CheckLockContradictoryOrder, Doc: "Reports inconsistent acquisition order for the same pair of locks."},
		}},
		{Analyzer: oncepolicy.Analyzer(), Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckOnceDiscardedWrapper, Doc: "Reports sync.Once function wrappers that are called and immediately discarded."},
		}},
		{Analyzer: syncmapatomicity.Analyzer(), Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckSyncMapNonAtomicClaim, Doc: "Reports separate sync.Map Load and Delete operations used to claim one value."},
		}},
		{Analyzer: taintpolicy.Analyzer(), OptIn: true, Checks: []analyzerbase.CheckInfo{
			{ID: analyzerbase.CheckTaintUntrustedSink, Doc: "Reports untrusted input that reaches a configured sensitive sink without validation."},
		}},
	}
}

func testingSpecs() []analyzerbase.AnalyzerSpec {
	return []analyzerbase.AnalyzerSpec{{
		Analyzer: testpolicy.Analyzer(), OptIn: true, SuggestedFix: true,
		Checks: []analyzerbase.CheckInfo{{ID: analyzerbase.CheckTestHelperMarker, Doc: "Reports test helpers that do not call Helper on every return path."}},
	}}
}
