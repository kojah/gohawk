package mixedcycles

import (
	"effectforward"
	"sync"
)

// Releasing before the wait, signaling before acquiring, and allowing a worker
// to release the parent's lock invalidate the necessary dependency.
func releasedBeforeDeferredJoin() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go effectforward.DeferredWorker(&mu, done)
	mu.Unlock()
	<-done
}

func earlySignalWithPrelude() {
	var mu, other sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go func() { other.Lock(); other.Unlock(); close(done); mu.Lock(); mu.Unlock() }()
	<-done
	mu.Unlock()
}

func workerReleasesParent() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go func() { mu.Unlock(); mu.Lock(); mu.Unlock(); close(done) }()
	<-done
}

func sharedPrelude(other *sync.Mutex) {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go effectforward.PreludeWorker(&mu, other, done)
	<-done
	mu.Unlock()
}

func deferredHelperJoin() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go effectforward.DeferredWorker(&mu, done)
	<-done // want "waiting for a worker while holding the mutex"
	mu.Unlock()
}

func balancedPreludeJoin() {
	var mu, other sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go effectforward.PreludeWorker(&mu, &other, done)
	<-done // want "waiting for a worker while holding the mutex"
	mu.Unlock()
}

func balancedPreludeChannel() {
	var mu, other sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go func() { other.Lock(); other.Unlock(); mu.Lock(); <-ch; mu.Unlock() }()
	ch <- 1 // want "channel operation holds the mutex"
	mu.Unlock()
}

func bufferedPreludeChannel() {
	var mu, other sync.Mutex
	ch := make(chan int, 1)
	mu.Lock()
	go func() { other.Lock(); other.Unlock(); mu.Lock(); <-ch; mu.Unlock() }()
	ch <- 1
	mu.Unlock()
}

func differentWorkerMutex() {
	var mu, other, third sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go effectforward.PreludeWorker(&third, &other, done)
	<-done
	mu.Unlock()
}
