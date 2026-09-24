package lockjoin

import (
	"errors"
	"sync"
)

var errInvalid = errors.New("invalid")

func validate(n int) error {
	if n < 0 {
		return errInvalid
	}
	return nil
}

// On the error path the parent waits for the worker before releasing the
// lock the worker needs.
func errorPathDeadlock(n int) error {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go worker(&mu, done)
	if err := validate(n); err != nil {
		<-done // want "waits for a worker that needs the held lock"
		mu.Unlock()
		return err
	}
	mu.Unlock()
	<-done
	return nil
}

// The lock and the wait depend on the same flag in opposite directions, so
// the only deadlocking combination cannot happen.
func correlatedLockAndWait(flag bool) {
	var mu sync.Mutex
	done := make(chan struct{})
	if flag {
		mu.Lock()
	}
	go worker(&mu, done)
	if !flag {
		<-done
	}
	if flag {
		mu.Unlock()
	}
}

// Two calls of one function may return related results, so a deadlock that
// needs them to differ is not reported.
func sameCallTwice(n int) {
	var mu sync.Mutex
	done := make(chan struct{})
	if validate(n) != nil {
		mu.Lock()
	}
	go worker(&mu, done)
	if validate(n) != nil {
		<-done
		mu.Unlock()
	}
}
