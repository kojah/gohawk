package lockjoin

import (
	"errors"
	"sync"
)

var errWorker = errors.New("worker failed")

// A deferred release still runs only after the wait. The function's detached
// recovery block is dead because no deferred call can recover, and the error
// result reaches the caller only after the cycle would already have blocked.
func blockedDeferredRelease() error {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	defer mu.Unlock()
	go worker(&mu, done)
	<-done // want "waits for a worker that needs the held lock"
	return nil
}

func blockedDeferredReleaseResults(fail bool) (int, error) {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	defer mu.Unlock()
	go worker(&mu, done)
	<-done // want "waits for a worker that needs the held lock"
	if fail {
		return 0, errWorker
	}
	return 1, nil
}

// Returning the channel hands it to the caller only after the wait.
func blockedReturnsChannel() chan struct{} {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	defer mu.Unlock()
	go worker(&mu, done)
	<-done // want "waits for a worker that needs the held lock"
	return done
}

// Releasing before the wait lets the worker acquire the mutex.
func releasedBeforeWaitWithResult() error {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go worker(&mu, done)
	mu.Unlock()
	<-done
	return nil
}

// A deferred closure that calls recover can resume at the recovery block and
// may do anything else before it releases. The protocol stays unknown.
func recoveredDeferredRelease() (err error) {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	defer func() {
		if recover() != nil {
			err = errWorker
		}
		mu.Unlock()
	}()
	go worker(&mu, done)
	<-done
	return nil
}
