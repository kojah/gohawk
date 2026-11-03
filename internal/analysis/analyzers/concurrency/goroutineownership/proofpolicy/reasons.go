package proofpolicy

// Reason identifies stable goroutine ownership evidence without encoding it as text.
type Reason uint8

const (
	None Reason = iota
	JoinProven
	DeferredJoinBeforeSpawn
	GuardedLocalJoin
	StopLifecycle
	ContextLifecycle
	LocallyCanceledContext
	ReceiverContext
	WaitGroupBound
	RelayDependency
	SynctestBubbleOwner
	CallerOrExternalOwner
	OwnershipTransfer
	OpaqueTransfer
	LoopJoinUnproven
	WorkerConsumesSignal
	FlagGuardedJoin
	BufferedSignal
	UnobservedSignal
	SignalCensusUnavailable
	SharedStorageSignal
	NoObligation
	UnownedReturn
	DoneBeforeCompletion
	SelectedReceiveEdge
	SelectedContextEdge
	CountedDrainEdge
	ProcessExitStopsWorker
	PreSpawnCensusCutoff
	ReceiveBudgetExhausted
	DiscoveryBudgetExhausted
	RetainedOwnerBudgetExhausted
	RelayDependencyBudgetExhausted
	// LabelSignalReceived starts the reasons for individual instruction labels.
	LabelSignalReceived
	LabelDirectJoin
	LabelSummaryJoin
	LabelHelper
	LabelPipePeer
	LabelTestingCleanup
	LabelStoredOutside
	LabelGoMockReturn
	LabelStoredTestingReceiver
	LabelSignalAggregateReceive
	LabelSelectSends
	LabelSent
	LabelStoredInMap
	LabelAppended
	LabelClosesRetainedOwner
	LabelDynamicCallee
	LabelCalleeWithoutBody
	LabelLaunchedHelper
	LabelReturnedTracked
	LabelReturnedProjection
	LabelReturnedContainment
	LabelPossibleJoin
	LabelOwnerLifecycle
	LabelPossibleSignalReceive
	SelectedPossibleReceiveEdge
	CountedPossibleDrainEdge
	FactoryOriginBudgetExhausted
	HelperCallBudgetExhausted
	SummaryJoinBudgetExhausted
	LabelCallUnavailable
	ReasonCount
)

var ownershipReasonCodes = [...]string{
	LabelCallUnavailable:           "call-classification-unavailable",
	SummaryJoinBudgetExhausted:     "summary-join-budget-exhausted",
	FactoryOriginBudgetExhausted:   "factory-origin-budget-exhausted",
	HelperCallBudgetExhausted:      "helper-call-budget-exhausted",
	PreSpawnCensusCutoff:           "pre-spawn-census-budget-exhausted",
	RelayDependencyBudgetExhausted: "relay-dependency-budget-exhausted",
	RetainedOwnerBudgetExhausted:   "retained-owner-budget-exhausted",
	DiscoveryBudgetExhausted:       "completion-discovery-budget-exhausted",
	ReceiveBudgetExhausted:         "worker-receive-budget-exhausted",
	None:                           "",
	JoinProven:                     "join-proven",
	DeferredJoinBeforeSpawn:        "deferred-join-before-spawn",
	GuardedLocalJoin:               "guarded-local-join",
	StopLifecycle:                  "stop-lifecycle",
	ContextLifecycle:               "context-lifecycle",
	LocallyCanceledContext:         "locally-canceled-context",
	ReceiverContext:                "receiver-context-lifecycle",
	WaitGroupBound:                 "waitgroup-bound-lifecycle",
	RelayDependency:                "relay-dependency-lifecycle",
	SynctestBubbleOwner:            "synctest-bubble-owner",
	CallerOrExternalOwner:          "caller-or-external-owner",
	OwnershipTransfer:              "ownership-transfer",
	OpaqueTransfer:                 "opaque-ownership-transfer",
	LoopJoinUnproven:               "loop-join-unproven",
	WorkerConsumesSignal:           "signal-consumed-by-worker",
	FlagGuardedJoin:                "flag-guarded-join",
	BufferedSignal:                 "buffered-completion-signal",
	SignalCensusUnavailable:        "signal-census-unavailable",
	UnobservedSignal:               "signal-never-observed",
	SharedStorageSignal:            "shared-storage-signal",
	NoObligation:                   "no-completion-obligation",
	UnownedReturn:                  "unowned-return",
	DoneBeforeCompletion:           "waitgroup-done-before-completion",
	SelectedReceiveEdge:            "selected-receive-edge",
	SelectedContextEdge:            "selected-context-edge",
	CountedDrainEdge:               "counted-drain-edge",
	ProcessExitStopsWorker:         "process-exit-stops-worker",
	LabelSignalReceived:            "signal-received",
	LabelDirectJoin:                "direct-join",
	LabelSummaryJoin:               "summary-join",
	LabelHelper:                    "helper-use",
	LabelPipePeer:                  "pipe-peer",
	LabelTestingCleanup:            "testing-cleanup",
	LabelStoredOutside:             "stored-outside-function",
	LabelGoMockReturn:              "gomock-return",
	LabelStoredTestingReceiver:     "stored-testing-receiver",
	LabelSignalAggregateReceive:    "receive-from-signal-aggregate",
	LabelSelectSends:               "select-sends-tracked",
	LabelSent:                      "sent-to-channel",
	LabelStoredInMap:               "stored-in-map",
	LabelAppended:                  "appended",
	LabelClosesRetainedOwner:       "closes-retained-owner",
	LabelDynamicCallee:             "dynamic-callee",
	LabelCalleeWithoutBody:         "callee-without-body",
	LabelLaunchedHelper:            "launched-helper",
	LabelReturnedTracked:           "returned-tracked-value",
	LabelReturnedProjection:        "returned-signal-projection",
	LabelReturnedContainment:       "returned-possible-owner",
	LabelPossibleJoin:              "possible-join-receiver",
	LabelOwnerLifecycle:            "owner-lifecycle-participation",
	LabelPossibleSignalReceive:     "possible-signal-receive",
	SelectedPossibleReceiveEdge:    "selected-possible-receive-edge",
	CountedPossibleDrainEdge:       "counted-possible-drain-edge",
}

func (reason Reason) String() string {
	if int(reason) >= len(ownershipReasonCodes) {
		return "invalid-goroutine-ownership-reason"
	}
	return ownershipReasonCodes[reason]
}
