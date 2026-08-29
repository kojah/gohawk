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

func conditionallyAcquired(index, other int) {
	if index != other {
		regressionFirst.Lock()
	}
	if index != other {
		regressionFirst.Unlock()
	}
}

func conditionalNil(pointer *int) {
	if pointer != nil {
		regressionFirst.Lock()
	}
	if pointer != nil {
		regressionFirst.Unlock()
	}
}

func conditionFromComputed(hashLock int) {
	refLock := computedLock(hashLock)
	if hashLock != refLock {
		regressionFirst.Lock()
	}
	if hashLock != refLock {
		regressionFirst.Unlock()
	}
}

func computedLock(value int) int { return value + 1 }

func transferredUnlock() {
	var mutex sync.Mutex
	mutex.Lock()
	done := make(chan struct{})
	go func() {
		mutex.Unlock()
		close(done)
	}()
	mutex.Lock()
	mutex.Unlock()
	<-done
}

func repeatedTransferredUnlock() {
	var mutex sync.Mutex
	mutex.Lock()
	done := make(chan struct{})
	go func() {
		for {
			mutex.Unlock()
			mutex.Lock()
			select {
			case done <- struct{}{}:
				return
			default:
			}
		}
	}()
	for range 10 {
		mutex.Lock()
		mutex.Unlock()
	}
	<-done
}

func nestedRepeatedTransferredUnlock() {
	func() {
		mutex := sync.Mutex{}
		mutex.Lock()
		done := make(chan struct{})
		go func() {
			for {
				mutex.Unlock()
				mutex.Lock()
				select {
				case done <- struct{}{}:
					return
				default:
				}
			}
		}()
		for range 10 {
			mutex.Lock()
			mutex.Unlock()
		}
		<-done
	}()
}

func conditionalGoroutineUnlock(release, fail bool) {
	var mutex sync.Mutex
	mutex.Lock()
	go func() {
		if release {
			mutex.Unlock()
		}
	}()
	if fail {
		return // want "lock lockorder.conditionalGoroutineUnlock:local:mutex is not released on this return path"
	}
	mutex.Unlock()
}
