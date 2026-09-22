package goroutineownership

import (
	"context"
	"testing"
)

func cancellationDoesNotDrainCompletionSend(ctx context.Context) {
	done := make(chan struct{})
	go func() { done <- struct{}{} }() // want "goroutine is not joined on every return path"
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func cancellationStillDrainsCompletionSend(ctx context.Context) {
	done := make(chan struct{})
	go func() { done <- struct{}{} }()
	select {
	case <-done:
	case <-ctx.Done():
		<-done
	}
}

func alternateCloseOrSendStillRequiresJoin(failed bool, cleanup func()) {
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer cleanup()
		if failed {
			close(done)
		} else {
			done <- struct{}{}
		}
	}()
}

func errorOnlyNotification(failed bool, success <-chan struct{}) {
	errc := make(chan struct{})
	go func() {
		if failed {
			close(errc)
			return
		}
	}()
	select {
	case <-errc:
	case <-success:
	}
}

func everyBranchCompletionStillRequiresJoin(failed bool, success <-chan struct{}) {
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		if failed {
			close(done)
			return
		}
		close(done)
	}()
	select {
	case <-done:
	case <-success:
	}
}

func emptyReceiveArmSharedReturn(timeout <-chan struct{}) {
	done := make(chan struct{})
	go func() { close(done) }()
	select {
	case <-done:
	case <-timeout:
		<-done
	}
}

func selectedCompletionOrExplicitJoin(timeout <-chan struct{}) {
	done := make(chan struct{})
	go func() { close(done) }()
	select {
	case <-done:
		return
	case <-timeout:
		<-done
		return
	}
}

func selectedCompletionOrFatal(t *testing.T, timeout <-chan struct{}) {
	done := make(chan struct{})
	go func() { close(done) }()
	select {
	case <-done:
		return
	case <-timeout:
		t.Fatal("timeout")
	}
}

func selectTimeoutDoesNotJoin(timeout <-chan struct{}) {
	done, work := make(chan struct{}), make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		<-work
		close(done)
	}()
	select {
	case <-done:
		return
	case <-timeout:
		return
	}
}

func selectDefaultDoesNotJoin() {
	done, work := make(chan struct{}), make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		<-work
		close(done)
	}()
	select {
	case <-done:
	default:
	}
}

func waitForCompletionOrTimeout(done, timeout <-chan struct{}) {
	select {
	case <-done:
	case <-timeout:
	}
}

func helperSelectTimeoutDoesNotJoin(timeout <-chan struct{}) {
	done, work := make(chan struct{}), make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		<-work
		close(done)
	}()
	waitForCompletionOrTimeout(done, timeout)
}
