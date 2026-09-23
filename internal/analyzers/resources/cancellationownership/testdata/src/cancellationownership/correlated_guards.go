package cancellationownership

// A cancel released under the same stable guard that created it is released
// on every path that guard admits; a different guard keeps the early return
// reachable.

import "context"

func cancelledUnderSameParameterGuard(parent context.Context, timed bool) {
	if !timed {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	_ = ctx
	if !timed {
		return
	}
	cancel()
}

func cancelledUnderDifferentParameterGuard(parent context.Context, timed, other bool) {
	if !timed {
		return
	}
	ctx, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	_ = ctx
	if !other {
		return
	}
	cancel()
}
