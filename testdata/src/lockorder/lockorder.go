package lockorder

import "sync"

var regressionFirst sync.Mutex
var regressionSecond sync.Mutex
var readLock sync.RWMutex

func regressionForward() {
	regressionFirst.Lock()
	defer regressionFirst.Unlock()
	regressionSecond.Lock()
	defer regressionSecond.Unlock()
}

func regressionReverse() {
	regressionSecond.Lock()
	defer regressionSecond.Unlock()
	regressionFirst.Lock() // want "contradictory lock order: regressionFirst and regressionSecond"
	defer regressionFirst.Unlock()
}

func missingUnlock(skip bool) {
	regressionFirst.Lock()
	if skip {
		return // want "lock regressionFirst is not released on this return path"
	}
	regressionFirst.Unlock()
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
	regressionFirst.Lock()
}
