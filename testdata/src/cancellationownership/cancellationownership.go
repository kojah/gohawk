package cancellationownership

import (
	"context"
	"os"
	"os/signal"
	"testing"
)

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

func deferredClosure(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	defer func() { cancel() }()
}

func cancelCause(parent context.Context) {
	_, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
}

func goroutineOwnsCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	go func() {
		defer cancel()
	}()
}

type owner struct {
	cancel   context.CancelFunc
	callback func()
}

func transferredToField(target *owner, parent context.Context) {
	_, cancel := context.WithCancel(parent)
	target.cancel = cancel
}

func transferredThroughCallback(target *owner, parent context.Context) {
	_, cancel := context.WithCancel(parent)
	target.callback = func() { cancel() }
}

func reassignedAndTransferredThroughCallback(target *owner, parent context.Context) {
	_, cancel := context.WithCancel(parent)
	cancel()
	_, cancel = context.WithCancel(parent)
	target.callback = func() { cancel() }
}

func leakedSignalContext(parent context.Context) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt) // want "cancel function from signal.NotifyContext is not called on every return path"
	_, _ = ctx, stop
}

func stoppedSignalContext(parent context.Context) {
	_, stop := signal.NotifyContext(parent, os.Interrupt)
	defer stop()
}

func afterFuncMayRun(parent context.Context) {
	_ = context.AfterFunc(parent, func() {})
}

func testCleanupOwnsCancel(t *testing.T, parent context.Context) {
	_, cancel := context.WithCancel(parent)
	t.Cleanup(cancel)
}

func mapOwnsCancel(cancels map[string]context.CancelFunc, parent context.Context) {
	_, cancel := context.WithCancel(parent)
	cancels["worker"] = cancel
}
