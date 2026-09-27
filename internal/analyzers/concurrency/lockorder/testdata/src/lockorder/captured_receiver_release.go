package lockorder

import "sync"

// A receiver captured by a closure is spilled to a cell written once, so the
// lock's owner and the argument handed to a releasing helper are two loads of
// one value. The helper still releases the caller's lock.

type capturedSubscription struct {
	mu    sync.Mutex
	state int
	later func()
}

// unsubscribeLocked releases s.mu on every return.
func (s *capturedSubscription) unsubscribeLocked() {
	s.state = 0
	s.mu.Unlock()
}

// maybeUnsubscribeLocked releases s.mu only when the state was set.
func (s *capturedSubscription) maybeUnsubscribeLocked() {
	if s.state == 0 {
		return
	}
	s.state = 0
	s.mu.Unlock()
}

// Accepted: the helper releases the lock the caller took.
func (s *capturedSubscription) resubscribe(fail bool) {
	s.later = func() { s.state++ }
	s.mu.Lock()
	if fail {
		s.unsubscribeLocked()
		return
	}
	s.mu.Unlock()
}

// The helper can return without releasing the lock.
func (s *capturedSubscription) resubscribeMaybe(fail bool) {
	s.later = func() { s.state++ }
	s.mu.Lock()
	if fail {
		s.maybeUnsubscribeLocked()
		return // want "lock `s\\.mu` is not released on this return path"
	}
	s.mu.Unlock()
}
