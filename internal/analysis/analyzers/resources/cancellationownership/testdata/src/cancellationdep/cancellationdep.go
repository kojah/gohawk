package cancellationdep

import "context"

func Invoke(cancel context.CancelFunc) { cancel() }

func MaybeInvoke(cancel context.CancelFunc, enabled bool) {
	if enabled {
		cancel()
	}
}

// NeverFails is an exported helper whose error result is always nil.
func NeverFails() error { return nil }

// InvokeLater calls cancel on another goroutine when enabled, so no caller
// can rely on it having run by the time this returns.
func InvokeLater(cancel context.CancelFunc, enabled bool) {
	if enabled {
		go cancel()
	}
}

// InvokeOther calls other, not cancel, when enabled.
func InvokeOther(cancel, other context.CancelFunc, enabled bool) {
	_ = cancel
	if enabled {
		other()
	}
}
