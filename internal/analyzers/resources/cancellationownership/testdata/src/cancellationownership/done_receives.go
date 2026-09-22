package cancellationownership

import (
	"context"
	"os/signal"
)

func selectedChildDone(parent context.Context, work <-chan error) error {
	ctx, cancel := context.WithCancel(parent)
	select {
	case err := <-work:
		cancel()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func receivedChildDone(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	_ = cancel
	<-ctx.Done()
}

func anotherSelectedCaseLeaks(parent context.Context, work <-chan error) error {
	ctx, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called"
	_ = cancel
	select {
	case err := <-work:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func merelyReadingDoneDoesNotCancel(parent context.Context) {
	ctx, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called"
	_, _ = cancel, ctx.Done()
}

func otherContextDoneDoesNotCancel(parent, other context.Context) {
	_, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called"
	_ = cancel
	<-other.Done()
}

func defaultSelectDoesNotCancel(parent context.Context) {
	ctx, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called"
	_ = cancel
	select {
	case <-ctx.Done():
	default:
	}
}

func receivedSignalDoesNotUnregister(parent context.Context) {
	ctx, stop := signal.NotifyContext(parent) // want "cancel function from signal.NotifyContext is not called"
	_ = stop
	<-ctx.Done()
}
