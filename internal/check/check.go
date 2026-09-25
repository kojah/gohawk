// Package check defines gohawk diagnostic identities and reporting behavior.
package check

// ID identifies one independently configurable diagnostic rule.
type ID string

const (
	CancellationRelease          ID = "cancellationownership/release"
	ChannelSendAfterClose        ID = "channelsafety/send-after-close"
	DeferCleanupInLoop           ID = "deferinloop/cleanup-lifetime"
	GoroutineJoin                ID = "goroutineownership/unjoined"
	ProducerLifecycleSend        ID = "producerlifecycle/abandoned-send"
	ProducerLifecycleStoppedLoop ID = "producerlifecycle/stopped-loop-send"
	ProducerLifecycleUnclosed    ID = "producerlifecycle/unclosed-range"
	ProcessWait                  ID = "processownership/missing-wait"
	ResourceRelease              ID = "resourcelifetime/missing-release"
	ResourceUseAfterRelease      ID = "resourcelifetime/use-after-release"
	ConcurrentCapture            ID = "concurrentcapture/shared-capture"
	LockMissingRelease           ID = "lockorder/missing-release"
	LockRecursiveAcquire         ID = "lockorder/recursive-acquire"
	LockContradictoryOrder       ID = "lockorder/contradictory-order"
	LockReadLockWrite            ID = "lockorder/read-lock-write"
)
