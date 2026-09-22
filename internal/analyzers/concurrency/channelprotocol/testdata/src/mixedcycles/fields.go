package mixedcycles

import "sync"

type state struct{ mu, other sync.Mutex }

func distinctFields() {
	var s state
	done := make(chan struct{})
	s.mu.Lock()
	go func() { s.other.Lock(); s.other.Unlock(); close(done) }()
	<-done
	s.mu.Unlock()
}

func releasedField() {
	var s state
	done := make(chan struct{})
	s.mu.Lock()
	go func() { s.mu.Lock(); s.mu.Unlock(); close(done) }()
	s.mu.Unlock()
	<-done
}

func borrowedField(s *state) {
	done := make(chan struct{})
	s.mu.Lock()
	go func() { s.mu.Lock(); s.mu.Unlock(); close(done) }()
	<-done
	s.mu.Unlock()
}

func resetFieldOwner() {
	var s state
	done := make(chan struct{})
	s.mu.Lock()
	go func() { s.mu.Lock(); s.mu.Unlock(); close(done) }()
	s = state{}
	<-done
	s.mu.Unlock()
}

func (s *state) finish(done chan struct{}) { s.mu.Lock(); s.mu.Unlock(); close(done) }

func fieldJoin() {
	var s state
	done := make(chan struct{})
	s.mu.Lock()
	go s.finish(done)
	<-done // want "waiting for a worker while holding the mutex"
	s.mu.Unlock()
}

func capturedField() {
	var s state
	done := make(chan struct{})
	s.mu.Lock()
	go func() { s.mu.Lock(); s.mu.Unlock(); close(done) }()
	<-done // want "waiting for a worker while holding the mutex"
	s.mu.Unlock()
}

type pointerState struct{ mu *sync.Mutex }

func pointerFieldUnknown() {
	s := pointerState{mu: new(sync.Mutex)}
	done := make(chan struct{})
	s.mu.Lock()
	go func() { s.mu.Lock(); s.mu.Unlock(); close(done) }()
	s.mu = new(sync.Mutex)
	<-done
	s.mu.Unlock()
}

func nestedFieldJoin() {
	var s struct{ inner state }
	done := make(chan struct{})
	s.inner.mu.Lock()
	go func() { s.inner.mu.Lock(); s.inner.mu.Unlock(); close(done) }()
	<-done // want "waiting for a worker while holding the mutex"
	s.inner.mu.Unlock()
}
