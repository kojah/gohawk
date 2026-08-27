package lockorder

import "sync"

var first sync.Mutex
var second sync.Mutex
var readLock sync.RWMutex

func forward() {
	first.Lock()
	defer first.Unlock()
	second.Lock()
	defer second.Unlock()
}

func reverse() {
	second.Lock()
	defer second.Unlock()
	first.Lock() // want "contradictory lock order: first and second"
	defer first.Unlock()
}

func missingUnlock(skip bool) {
	first.Lock()
	if skip {
		return // want "lock first is not released on this return path"
	}
	first.Unlock()
}

func missingReadUnlock(skip bool) {
	readLock.RLock()
	if skip {
		return // want "lock readLock is not released on this return path"
	}
	readLock.RUnlock()
}

func deferredUnlock(skip bool) {
	first.Lock()
	defer first.Unlock()
	if skip {
		return
	}
}

// Returning with a lock held on every path may intentionally transfer the
// critical section to the caller, so the analyzer stays quiet without evidence
// of a local release policy.
func intentionallyHeld() {
	first.Lock()
}
