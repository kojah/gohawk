// Package check defines gohawk diagnostic identities and reporting behavior.
package check

// ID identifies one independently configurable diagnostic rule.
type ID string

const (
	CancellationRelease      ID = "cancellationownership/release"
	BorrowedStorageOwner     ID = "borrowedstorage/overlapping-owner"
	ChannelSendAfterClose    ID = "channelsafety/send-after-close"
	ChannelDoubleClose       ID = "channelsafety/double-close"
	DeferCleanupInLoop       ID = "deferinloop/cleanup-lifetime"
	GoroutineJoin            ID = "goroutineownership/unjoined"
	ProducerLifecycleSend    ID = "producerlifecycle/abandoned-send"
	ChannelProtocolBlocked   ID = "channelprotocol/blocked-operation"
	ChannelProtocolLockJoin  ID = "channelprotocol/lock-and-join"
	ChannelProtocolLockCycle ID = "channelprotocol/channel-lock-cycle"
	ProcessWait              ID = "processownership/missing-wait"
	ProcessDetached          ID = "processownership/detached"
	ResourceRelease          ID = "resourcelifetime/missing-release"
	ResourceUseAfterRelease  ID = "resourcelifetime/use-after-release"
	ConcurrentCapture        ID = "concurrentcapture/shared-capture"
	ErrorMismatchedInline    ID = "inlineerror/mismatched-condition"
	EvaluationOrder          ID = "evalorder/operand-mutation"
	LockMissingRelease       ID = "lockorder/missing-release"
	LockRecursiveAcquire     ID = "lockorder/recursive-acquire"
	LockContradictoryOrder   ID = "lockorder/contradictory-order"
	LockReadLockWrite        ID = "lockorder/read-lock-write"
	LockMismatchedRelease    ID = "lockorder/mismatched-release"
	LockDiscardedTryLock     ID = "lockorder/discarded-trylock"
	OnceDiscardedWrapper     ID = "oncepolicy/discarded-wrapper"
	SyncMapNonAtomicClaim    ID = "syncmapatomicity/non-atomic-claim"
)
