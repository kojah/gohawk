package cancellationownership

import "context"

func leaked(parent context.Context) {
	ctx, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	_, _ = ctx, cancel
}

func canceled(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	defer cancel()
}

func conditional(parent context.Context, stop bool) {
	_, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	if stop {
		cancel()
	}
}

func transferred(parent context.Context) context.CancelFunc {
	_, cancel := context.WithCancel(parent)
	return cancel
}
