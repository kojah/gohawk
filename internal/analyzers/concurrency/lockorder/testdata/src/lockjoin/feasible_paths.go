package lockjoin

import (
	"errors"
	"sync"

	"lockjoinhelper"
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

// The worker takes the lock only when enabled, and the parent waits only when
// enabled: the worker's condition binds to the parent's own flag, so the
// deadlocking combination is one feasible execution.
func correlatedWorkerDeadlock(enabled bool) {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go conditionalWorker(&mu, done, enabled)
	if enabled {
		<-done // want "waits for a worker that needs the held lock"
	}
	mu.Unlock()
}

// The parent waits while holding the lock only when the worker skips it, so
// the combination that would deadlock needs enabled to be true and false at
// once. Without binding the worker's condition to the parent's flag, this was
// only unknown.
func correlatedWorkerSafe(enabled bool) {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go conditionalWorker(&mu, done, enabled)
	if !enabled {
		<-done
	}
	mu.Unlock()
}

type lockError struct{}

func (*lockError) Error() string { return "lock failed" }

// acquireLock locks only on success: its error path returns a fresh error and
// its success path returns nil, so a caller's test of the result tells which.
func acquireLock(mu *sync.Mutex, n int) error {
	if n < 0 {
		return &lockError{}
	}
	mu.Lock()
	return nil
}

// On success the lock is held while the parent waits for a worker that needs
// it; the helper's result ties the caller's error check to that path.
func resultConditionedDeadlock(n int) error {
	var mu sync.Mutex
	done := make(chan struct{})
	if err := acquireLock(&mu, n); err != nil {
		return err
	}
	go worker(&mu, done)
	<-done // want "waits for a worker that needs the held lock"
	mu.Unlock()
	return nil
}

// The parent waits only when acquisition failed, and a failed acquisition
// holds nothing, so the combination that would deadlock is impossible.
func resultConditionedSafe(n int) {
	var mu sync.Mutex
	done := make(chan struct{})
	err := acquireLock(&mu, n)
	go worker(&mu, done)
	if err != nil {
		<-done
		return
	}
	mu.Unlock()
}

// The same shape through an imported helper: the published alternatives tie
// the caller's error check to the helper's locking path.
func importedResultDeadlock(n int) error {
	var mu sync.Mutex
	done := make(chan struct{})
	if err := lockjoinhelper.Acquire(&mu, n); err != nil {
		return err
	}
	go worker(&mu, done)
	<-done // want "waits for a worker that needs the held lock"
	mu.Unlock()
	return nil
}
