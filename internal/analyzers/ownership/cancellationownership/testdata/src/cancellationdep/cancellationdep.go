package cancellationdep

import "context"

func Invoke(cancel context.CancelFunc) { cancel() }

func MaybeInvoke(cancel context.CancelFunc, enabled bool) {
	if enabled {
		cancel()
	}
}
