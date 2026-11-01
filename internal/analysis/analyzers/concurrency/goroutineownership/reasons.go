package goroutineownership

import "github.com/kojah/gohawk/internal/analysis/analyzers/concurrency/goroutineownership/proofpolicy"

type goroutineOwnershipReason = proofpolicy.Reason

const (
	reasonNone                           = proofpolicy.None
	reasonJoinProven                     = proofpolicy.JoinProven
	reasonDeferredJoinBeforeSpawn        = proofpolicy.DeferredJoinBeforeSpawn
	reasonGuardedLocalJoin               = proofpolicy.GuardedLocalJoin
	reasonStopLifecycle                  = proofpolicy.StopLifecycle
	reasonContextLifecycle               = proofpolicy.ContextLifecycle
	reasonLocallyCanceledContext         = proofpolicy.LocallyCanceledContext
	reasonReceiverContext                = proofpolicy.ReceiverContext
	reasonWaitGroupBound                 = proofpolicy.WaitGroupBound
	reasonRelayDependency                = proofpolicy.RelayDependency
	reasonSynctestBubbleOwner            = proofpolicy.SynctestBubbleOwner
	reasonCallerOrExternalOwner          = proofpolicy.CallerOrExternalOwner
	reasonOwnershipTransfer              = proofpolicy.OwnershipTransfer
	reasonOpaqueTransfer                 = proofpolicy.OpaqueTransfer
	reasonLoopJoinUnproven               = proofpolicy.LoopJoinUnproven
	reasonWorkerConsumesSignal           = proofpolicy.WorkerConsumesSignal
	reasonFlagGuardedJoin                = proofpolicy.FlagGuardedJoin
	reasonBufferedSignal                 = proofpolicy.BufferedSignal
	reasonUnobservedSignal               = proofpolicy.UnobservedSignal
	reasonSignalCensusUnavailable        = proofpolicy.SignalCensusUnavailable
	reasonSharedStorageSignal            = proofpolicy.SharedStorageSignal
	reasonNoObligation                   = proofpolicy.NoObligation
	reasonUnownedReturn                  = proofpolicy.UnownedReturn
	reasonDoneBeforeCompletion           = proofpolicy.DoneBeforeCompletion
	reasonSelectedReceiveEdge            = proofpolicy.SelectedReceiveEdge
	reasonSelectedContextEdge            = proofpolicy.SelectedContextEdge
	reasonCountedDrainEdge               = proofpolicy.CountedDrainEdge
	reasonProcessExitStopsWorker         = proofpolicy.ProcessExitStopsWorker
	reasonPreSpawnCensusCutoff           = proofpolicy.PreSpawnCensusCutoff
	reasonReceiveBudgetExhausted         = proofpolicy.ReceiveBudgetExhausted
	reasonDiscoveryBudgetExhausted       = proofpolicy.DiscoveryBudgetExhausted
	reasonRetainedOwnerBudgetExhausted   = proofpolicy.RetainedOwnerBudgetExhausted
	reasonRelayDependencyBudgetExhausted = proofpolicy.RelayDependencyBudgetExhausted
	reasonLabelSignalReceived            = proofpolicy.LabelSignalReceived
	reasonLabelDirectJoin                = proofpolicy.LabelDirectJoin
	reasonLabelSummaryJoin               = proofpolicy.LabelSummaryJoin
	reasonLabelHelper                    = proofpolicy.LabelHelper
	reasonLabelPipePeer                  = proofpolicy.LabelPipePeer
	reasonLabelTestingCleanup            = proofpolicy.LabelTestingCleanup
	reasonLabelStoredOutside             = proofpolicy.LabelStoredOutside
	reasonLabelGoMockReturn              = proofpolicy.LabelGoMockReturn
	reasonLabelStoredTestingReceiver     = proofpolicy.LabelStoredTestingReceiver
	reasonLabelSignalAggregateReceive    = proofpolicy.LabelSignalAggregateReceive
	reasonLabelSelectSends               = proofpolicy.LabelSelectSends
	reasonLabelSent                      = proofpolicy.LabelSent
	reasonLabelStoredInMap               = proofpolicy.LabelStoredInMap
	reasonLabelAppended                  = proofpolicy.LabelAppended
	reasonLabelClosesRetainedOwner       = proofpolicy.LabelClosesRetainedOwner
	reasonLabelDynamicCallee             = proofpolicy.LabelDynamicCallee
	reasonLabelCalleeWithoutBody         = proofpolicy.LabelCalleeWithoutBody
	reasonLabelLaunchedHelper            = proofpolicy.LabelLaunchedHelper
	reasonLabelReturnedTracked           = proofpolicy.LabelReturnedTracked
	reasonLabelReturnedProjection        = proofpolicy.LabelReturnedProjection
	reasonLabelReturnedContainment       = proofpolicy.LabelReturnedContainment
	reasonLabelPossibleJoin              = proofpolicy.LabelPossibleJoin
	reasonLabelOwnerLifecycle            = proofpolicy.LabelOwnerLifecycle
	reasonLabelPossibleSignalReceive     = proofpolicy.LabelPossibleSignalReceive
	reasonSelectedPossibleReceiveEdge    = proofpolicy.SelectedPossibleReceiveEdge
	reasonCountedPossibleDrainEdge       = proofpolicy.CountedPossibleDrainEdge
	reasonFactoryOriginBudgetExhausted   = proofpolicy.FactoryOriginBudgetExhausted
	reasonHelperCallBudgetExhausted      = proofpolicy.HelperCallBudgetExhausted
	reasonSummaryJoinBudgetExhausted     = proofpolicy.SummaryJoinBudgetExhausted
	reasonLabelCallUnavailable           = proofpolicy.LabelCallUnavailable
	goroutineOwnershipReasonCount        = proofpolicy.ReasonCount
)

// summaryJoinReason classifies this check's evidence independently of upstream model failures.
type summaryJoinReason uint8

const (
	summaryJoinNone summaryJoinReason = iota
	summaryJoinConcurrencyJoinBudgetExhausted
	summaryJoinConcurrencyJoinNotApplicable
	summaryJoinConcurrencyJoinNotSynchronous
	summaryJoinConcurrencySummaryJoin
	summaryJoinConcurrencySummaryNoExactJoin
	summaryJoinReasonCount
)

func (reason summaryJoinReason) String() string {
	switch reason {
	case summaryJoinNone:
		return ""
	case summaryJoinConcurrencyJoinBudgetExhausted:
		return "concurrency-join-budget-exhausted"
	case summaryJoinConcurrencyJoinNotApplicable:
		return "concurrency-join-not-applicable"
	case summaryJoinConcurrencyJoinNotSynchronous:
		return "concurrency-join-not-synchronous"
	case summaryJoinConcurrencySummaryJoin:
		return "concurrency-summary-join"
	case summaryJoinConcurrencySummaryNoExactJoin:
		return "concurrency-summary-no-exact-join"
	default:
		return "invalid-goroutineownership-reason"
	}
}
