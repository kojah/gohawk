package cancellationownership

// A return reached only through a call that never returns does not skip the
// cancel; a wrapper that may return keeps the early return reachable.

import (
	"context"
	"os"
)

func fatal(message string) {
	_ = message
	os.Exit(1)
}

func fatalIf(fail bool) {
	if fail {
		os.Exit(1)
	}
}

func cancelledUnlessFatal(parent context.Context, fail bool) {
	ctx, cancel := context.WithCancel(parent)
	_ = ctx
	if fail {
		fatal("failed")
		return
	}
	cancel()
}

func cancelledUnlessMaybeFatal(parent context.Context, fail, other bool) {
	ctx, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	_ = ctx
	if fail {
		fatalIf(other)
		return
	}
	cancel()
}
