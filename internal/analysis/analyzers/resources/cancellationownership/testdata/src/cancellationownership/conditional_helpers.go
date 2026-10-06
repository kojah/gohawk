package cancellationownership

import "context"

func cancelIfReady(cancel context.CancelFunc, ready bool) bool {
	if !ready {
		return false
	}
	cancel()
	return true
}

func conditionalHelperCleanup(ready bool) {
	_, cancel := context.WithCancel(context.Background())
	if cancelIfReady(cancel, ready) {
		return
	}
	cancel()
}

// Conditional cleanup must not excuse a return before the helper is called.
func conditionalHelperTooLate(ready, early bool) {
	_, cancel := context.WithCancel(context.Background()) // want "cancel function from context.WithCancel is not called"
	if early {
		return
	}
	if cancelIfReady(cancel, ready) {
		return
	}
	cancel()
}
