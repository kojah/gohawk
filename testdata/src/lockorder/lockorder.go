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

type guardedState struct {
	mutex sync.Mutex
}

var firstState guardedState
var secondState guardedState

var interfaceFirst sync.Mutex
var interfaceSecond sync.Mutex

func nestedFieldForward() {
	firstState.mutex.Lock()
	defer firstState.mutex.Unlock()
	secondState.mutex.Lock()
	defer secondState.mutex.Unlock()
}

func nestedFieldReverse() {
	secondState.mutex.Lock()
	defer secondState.mutex.Unlock()
	firstState.mutex.Lock() // want "contradictory lock order: firstState.mutex and secondState.mutex"
	defer firstState.mutex.Unlock()
}

func localFieldMissingUnlock(skip bool) {
	state := new(guardedState)
	state.mutex.Lock()
	if skip {
		return // want "lock lockorder.localFieldMissingUnlock:local:new.mutex is not released on this return path"
	}
	state.mutex.Unlock()
}

func distinctDynamicFieldLocks(first, second *guardedState) {
	first.mutex.Lock()
	defer first.mutex.Unlock()
	second.mutex.Lock()
	defer second.mutex.Unlock()
}

func conditionalDeferredUnlock(fail bool) {
	var mutex sync.Mutex
	unlocked := false
	mutex.Lock()
	defer func() {
		if !unlocked {
			mutex.Unlock()
		}
	}()
	if fail {
		return
	}
	mutex.Unlock()
	unlocked = true
}

func interfaceForward() {
	var first sync.Locker = &interfaceFirst
	var second sync.Locker = &interfaceSecond
	first.Lock()
	defer first.Unlock()
	second.Lock()
	defer second.Unlock()
}

func interfaceReverse() {
	var first sync.Locker = &interfaceFirst
	var second sync.Locker = &interfaceSecond
	second.Lock()
	defer second.Unlock()
	first.Lock() // want "contradictory lock order: interfaceFirst and interfaceSecond"
	defer first.Unlock()
}

func unknownInterfaceLock(lock sync.Locker) {
	lock.Lock()
	defer lock.Unlock()
}

func knownInterfaceMissingUnlock(skip bool) {
	var lock sync.Locker = &interfaceFirst
	lock.Lock()
	if skip {
		return // want "lock interfaceFirst is not released on this return path"
	}
	lock.Unlock()
}

func ambiguousInterfaceLock(reverse, skip bool) {
	var lock sync.Locker
	if reverse {
		lock = &interfaceFirst
	} else {
		lock = &interfaceSecond
	}
	lock.Lock()
	if skip {
		return
	}
	lock.Unlock()
}

type tryLocker interface {
	sync.Locker
	TryLock() bool
}

func convertedInterfaceMissingUnlock(skip bool) {
	var extended tryLocker = &interfaceFirst
	var lock sync.Locker = extended
	lock.Lock()
	if skip {
		return // want "lock interfaceFirst is not released on this return path"
	}
	lock.Unlock()
}

func sameOriginInterfaceLock(branch, skip bool) {
	var lock sync.Locker
	if branch {
		lock = &interfaceFirst
	} else {
		lock = &interfaceFirst
	}
	lock.Lock()
	if skip {
		return // want "lock interfaceFirst is not released on this return path"
	}
	lock.Unlock()
}
