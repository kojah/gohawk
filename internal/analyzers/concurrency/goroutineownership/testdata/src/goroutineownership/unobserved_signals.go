package goroutineownership

import (
	"errors"
	"io"
)

// A channel the worker only closes, and that nothing ever receives from, is
// not a completion protocol: no code waits for it and close never blocks.
// A close observed on only some paths, or a send, still carries the join.

// Accepted: a drain loop closes a channel nobody reads.
func drainClosesUnobservedChannel(r io.Reader) {
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		buffer := make([]byte, 8)
		for {
			if _, err := r.Read(buffer); err != nil {
				return
			}
		}
	}()
	waitGroupWork()
}

// Accepted: an early return cannot skip a receive that never exists.
func displayClosesUnobservedDone(updates chan int, fail bool) error {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range updates {
		}
	}()
	if fail {
		return errors.New("build failed")
	}
	close(updates)
	return nil
}

// The parent receives the close on one path, so the other path skips it.
func closeObservedOnOnePath(fail bool) error {
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		waitGroupWork()
	}()
	if fail {
		return errors.New("failed")
	}
	<-done
	return nil
}

// A send nobody receives blocks the worker forever.
func sendNeverReceived() {
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		waitGroupWork()
		done <- struct{}{}
	}()
}
