package lockjoin

import (
	"sync"

	"lockjoinhelper"
)

func worker(mu *sync.Mutex, done chan<- struct{}) {
	mu.Lock()
	mu.Unlock()
	close(done)
}

func blockedLocal() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go worker(&mu, done)
	<-done // want "waits for a worker that needs the held lock"
	mu.Unlock()
}

func blockedImported() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go lockjoinhelper.Finish(&mu, done)
	<-done // want "waits for a worker that needs the held lock"
	mu.Unlock()
}

// Releasing before the wait allows the worker to acquire the mutex.
func releasedBeforeWait() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go worker(&mu, done)
	mu.Unlock()
	<-done
}

// The worker's lock is a distinct allocation.
func distinctMutex() {
	var parent, child sync.Mutex
	done := make(chan struct{})
	parent.Lock()
	go worker(&child, done)
	<-done
	parent.Unlock()
}

// Signalling before acquiring the mutex breaks the dependency.
func signalsFirst(mu *sync.Mutex, done chan<- struct{}) {
	close(done)
	mu.Lock()
	mu.Unlock()
}

func signalBeforeLock() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go signalsFirst(&mu, done)
	<-done
	mu.Unlock()
}

// A caller-owned mutex might be deliberately unlocked by another participant.
func externallyOwned(mu *sync.Mutex) {
	done := make(chan struct{})
	mu.Lock()
	go worker(mu, done)
	<-done
	mu.Unlock()
}

// A second goroutine may satisfy the wait; the one-worker summary is unavailable.
func alternateSender() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go worker(&mu, done)
	go func() { done <- struct{}{} }()
	<-done
	mu.Unlock()
}

// Divergent effects in the worker cannot establish a must-close obligation.
func conditionalWorker(mu *sync.Mutex, done chan<- struct{}, enabled bool) {
	if enabled {
		mu.Lock()
		mu.Unlock()
	}
	close(done)
}

func conditionalLock(enabled bool) {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go conditionalWorker(&mu, done, enabled)
	<-done
	mu.Unlock()
}
