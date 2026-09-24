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

func launchWorker(mu *sync.Mutex, done chan<- struct{}) { go worker(mu, done) }

func blockedThroughHelper() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	launchWorker(&mu, done)
	<-done // want "waits for a worker that needs the held lock"
	mu.Unlock()
}

func blockedThroughImportedHelper() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	lockjoinhelper.Launch(&mu, done)
	<-done // want "waits for a worker that needs the held lock"
	mu.Unlock()
}

func releasedBeforeHelperWait() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	launchWorker(&mu, done)
	mu.Unlock()
	<-done
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

// A second goroutine may satisfy the wait, even when the other worker cannot.
func alternateSender() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go worker(&mu, done)
	go func() { done <- struct{}{} }()
	<-done
	mu.Unlock()
}

// An unrelated child cannot satisfy the receive or release the held lock.
func blockedWithUnrelatedChild() {
	var mu sync.Mutex
	done := make(chan struct{})
	other := make(chan struct{})
	mu.Lock()
	go worker(&mu, done)
	go func() { close(other) }()
	<-done // want "waits for a worker that needs the held lock"
	mu.Unlock()
}

// Either child could close done, but each must acquire the held lock first.
func blockedWithTwoClosers() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go worker(&mu, done)
	go worker(&mu, done)
	<-done // want "waits for a worker that needs the held lock"
	mu.Unlock()
}

// Unlocking from another goroutine can let the signaling worker proceed.
func alternateUnlocker() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go worker(&mu, done)
	go func() { mu.Unlock() }()
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
