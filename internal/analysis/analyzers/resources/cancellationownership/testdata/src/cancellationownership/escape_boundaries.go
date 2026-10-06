package cancellationownership

import "context"

// A local cell exposed before its write may be observed by retained code.
// This is unknown retention, not a proven cleanup or ownership transfer.
// cancelStoredInPrivateLocal covers the corresponding unexposed diagnostic.
func cancelStoredInExposedLocal(parent context.Context, publish func(*context.CancelFunc), acquire func() bool) {
	var stored context.CancelFunc
	publish(&stored)
	if acquire() {
		_, cancel := context.WithCancel(parent)
		stored = cancel
	}
	_ = stored
}
