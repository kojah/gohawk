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
