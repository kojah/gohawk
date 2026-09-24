// Package check defines gohawk diagnostic identities and reporting behavior.
package check

// ID identifies one independently configurable diagnostic rule.
type ID string

const (
	CancellationRelease     ID = "cancellationownership/release"
	ChannelSendAfterClose   ID = "channelsafety/send-after-close"
	DeferCleanupInLoop      ID = "deferinloop/cleanup-lifetime"
	GoroutineJoin           ID = "goroutineownership/unjoined"
	ProducerLifecycleSend   ID = "producerlifecycle/abandoned-send"
	ProcessWait             ID = "processownership/missing-wait"
	ResourceRelease         ID = "resourcelifetime/missing-release"
	ResourceUseAfterRelease ID = "resourcelifetime/use-after-release"
	ConcurrentCapture       ID = "concurrentcapture/shared-capture"
	ErrorMismatchedInline   ID = "inlineerror/mismatched-condition"
	EvaluationOrder         ID = "evalorder/operand-mutation"
	LockMissingRelease      ID = "lockorder/missing-release"
	LockRecursiveAcquire    ID = "lockorder/recursive-acquire"
	LockContradictoryOrder  ID = "lockorder/contradictory-order"
	LockAndJoin             ID = "lockorder/lock-and-join"
	LockChannelCycle        ID = "lockorder/channel-lock-cycle"
	LockReadLockWrite       ID = "lockorder/read-lock-write"
	LockMismatchedRelease   ID = "lockorder/mismatched-release"
	OnceDiscardedWrapper    ID = "oncepolicy/discarded-wrapper"
	NilArgumentDereference  ID = "nilargument/dereferenced-nil"
)
