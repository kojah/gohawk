package mixedcycles

import "sync"

// These cases define the precision boundary before mixed dependency proofs:
// release before waiting, different locks, buffering, early completion,
// alternative participants, caller-owned resources, and conditional progress.
// Unsupported loops/selects and mutable or opaque resources must stay unknown.
func releaseBeforeJoin() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go func() { mu.Lock(); mu.Unlock(); close(done) }()
	mu.Unlock()
	<-done
}

func differentLocks() {
	var first, second sync.Mutex
	done := make(chan struct{})
	first.Lock()
	go func() { second.Lock(); second.Unlock(); close(done) }()
	<-done
	first.Unlock()
}

func signalBeforeLock() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go func() { close(done); mu.Lock(); mu.Unlock() }()
	<-done
	mu.Unlock()
}

func bufferedSend() {
	var mu sync.Mutex
	ch := make(chan int, 1)
	mu.Lock()
	go func() { mu.Lock(); <-ch; mu.Unlock() }()
	ch <- 1
	mu.Unlock()
}

func receiveBeforeLock() {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go func() { <-ch; mu.Lock(); mu.Unlock() }()
	ch <- 1
	mu.Unlock()
}

func anotherReceiver() {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go func() { mu.Lock(); <-ch; mu.Unlock() }()
	go func() { <-ch }()
	ch <- 1
	mu.Unlock()
}

func externalMutex(mu *sync.Mutex) {
	done := make(chan struct{})
	mu.Lock()
	go func() { mu.Lock(); mu.Unlock(); close(done) }()
	<-done
	mu.Unlock()
}

func externalDirectMutex(mu *sync.Mutex) {
	done := make(chan struct{})
	mu.Lock()
	go mutexWorker(mu, done)
	<-done
	mu.Unlock()
}

func mutexWorker(mu *sync.Mutex, done chan struct{}) { mu.Lock(); mu.Unlock(); close(done) }

func conditionalWorker(skip bool) {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go func() {
		if !skip {
			mu.Lock()
			mu.Unlock()
		}
		close(done)
	}()
	<-done
	mu.Unlock()
}

func cancellableSend(stop <-chan struct{}) {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go func() { mu.Lock(); <-ch; mu.Unlock() }()
	select {
	case ch <- 1:
	case <-stop:
	}
	mu.Unlock()
}

func earlyDone() {
	var mu sync.Mutex
	var group sync.WaitGroup
	group.Add(1)
	mu.Lock()
	go func() { group.Done(); mu.Lock(); mu.Unlock() }()
	group.Wait()
	mu.Unlock()
}

func sharedReadLocks() {
	var mu sync.RWMutex
	done := make(chan struct{})
	mu.RLock()
	go func() { mu.RLock(); mu.RUnlock(); close(done) }()
	<-done
	mu.RUnlock()
}

func resetMutex() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	mu = sync.Mutex{}
	go func() { mu.Lock(); mu.Unlock(); close(done) }()
	<-done
	mu.Unlock()
}
