package concurrentcapture

// Capture reasons retain the distinction between a proven guard and an
// unsupported shape. Only their presentation is textual.
type captureReason uint8

const (
	reasonNone captureReason = iota
	reasonRepeatedWrite
	reasonLockFallbackUnknown
	reasonWorkerGuardUnknown
	reasonChannelGuardUnknown
	reasonUnguardedWrite
	reasonWorkerOrderUnknown
	reasonMutationSiteUnknown
	reasonHelperEffectsUnknown
	reasonLockIdentityUnknown
	reasonLockHeld
	reasonNoLockHeld
	captureReasonCount
)

func (reason captureReason) String() string {
	switch reason {
	case reasonNone:
		return ""
	case reasonRepeatedWrite:
		return "capture-repeated-write"
	case reasonLockFallbackUnknown:
		return "capture-lock-fallback-unknown"
	case reasonWorkerGuardUnknown:
		return "capture-worker-guard-unknown"
	case reasonChannelGuardUnknown:
		return "capture-channel-guard-unknown"
	case reasonUnguardedWrite:
		return "capture-unguarded-write"
	case reasonWorkerOrderUnknown:
		return "capture-worker-order-unknown"
	case reasonMutationSiteUnknown:
		return "capture-mutation-site-unknown"
	case reasonHelperEffectsUnknown:
		return "capture-helper-effects-unknown"
	case reasonLockIdentityUnknown:
		return "capture-lock-identity-unknown"
	case reasonLockHeld:
		return "capture-lock-held"
	case reasonNoLockHeld:
		return "capture-no-lock-held"
	default:
		return "invalid-capture-reason"
	}
}
