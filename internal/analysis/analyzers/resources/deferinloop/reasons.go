package deferinloop

// Defer reasons describe instruction classification and loop-flow outcomes.
// Resource status remains separate; an unset reason proves no cleanup.
type deferReason uint8

const (
	reasonNone deferReason = iota
	reasonDeferredCleanup
	reasonRetainedBeforeDefer
	reasonLiveAtBackedge
	reasonIteratorExhausted
	reasonNoLiveBackedge
	reasonArgumentCarriesResource
	reasonResourceTransferred
	reasonResourceCapturedOrStored
	reasonExplicitCleanup
	reasonWrapperPassedToCallee
	reasonCalleeReleasesArgument
	reasonUnsummarizedCalleeUse
	reasonLifetimeUnknownAtBackedge
	reasonDeferLocationUnknown
	deferReasonCount
)

func (reason deferReason) String() string {
	switch reason {
	case reasonNone:
		return ""
	case reasonDeferredCleanup:
		return "deferred-cleanup-in-loop"
	case reasonRetainedBeforeDefer:
		return "retained-before-defer"
	case reasonLiveAtBackedge:
		return "live-at-backedge"
	case reasonIteratorExhausted:
		return "iterator-exhausted"
	case reasonNoLiveBackedge:
		return "no-live-backedge"
	case reasonArgumentCarriesResource:
		return "argument-carries-resource"
	case reasonResourceTransferred:
		return "resource-transferred"
	case reasonResourceCapturedOrStored:
		return "resource-captured-or-stored"
	case reasonExplicitCleanup:
		return "explicit-cleanup"
	case reasonWrapperPassedToCallee:
		return "wrapper-passed-to-callee"
	case reasonCalleeReleasesArgument:
		return "callee-releases-argument"
	case reasonUnsummarizedCalleeUse:
		return "unsummarized-callee-uses-resource"
	case reasonLifetimeUnknownAtBackedge:
		return "lifetime-unknown-at-backedge"
	case reasonDeferLocationUnknown:
		return "defer-location-unknown"
	default:
		return "invalid-defer-reason"
	}
}
