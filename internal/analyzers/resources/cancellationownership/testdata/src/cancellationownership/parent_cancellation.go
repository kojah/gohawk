package cancellationownership

import (
	"context"
	"os"
	"os/signal"
	"time"
)

func childOwnedByDeferredParent() {
	parent, cancel := context.WithCancelCause(context.Background())
	child, _ := context.WithTimeoutCause(parent, time.Hour, context.DeadlineExceeded)
	defer func() { cancel(context.Canceled) }()
	_ = child
}

func childOwnedByReturnedParent() (context.Context, context.CancelFunc) {
	parent, cancel := context.WithCancel(context.Background())
	child, _ := context.WithTimeout(parent, time.Hour)
	return child, cancel
}

func childOwnedByDirectParent() {
	parent, cancel := context.WithCancel(context.Background())
	child, _ := context.WithTimeout(parent, time.Hour)
	_ = child
	cancel()
}

func unrelatedParentDoesNotReleaseChild() {
	parent, releaseParent := context.WithCancel(context.Background()) // want "cancel function from context.WithCancel is not called"
	other, releaseOther := context.WithCancel(context.Background())
	child, _ := context.WithTimeout(parent, time.Hour) // want "cancel function from context.WithTimeout is not called"
	_ = child
	_ = other
	releaseOther()
	_ = releaseParent
}

func conditionalParentDoesNotReleaseChild(release bool) {
	parent, cancel := context.WithCancel(context.Background()) // want "cancel function from context.WithCancel is not called"
	child, _ := context.WithTimeout(parent, time.Hour)         // want "cancel function from context.WithTimeout is not called"
	_ = child
	if release {
		cancel()
	}
}

func parentDoesNotUnregisterSignal() {
	parent, cancel := context.WithCancel(context.Background())
	_, _ = signal.NotifyContext(parent, os.Interrupt) // want "cancel function from signal.NotifyContext is not called"
	cancel()
}

func detachedChildDoesNotFollowParent() {
	parent, cancel := context.WithCancel(context.Background())
	_, _ = context.WithTimeout(context.WithoutCancel(parent), time.Hour) // want "cancel function from context.WithTimeout is not called"
	cancel()
}
