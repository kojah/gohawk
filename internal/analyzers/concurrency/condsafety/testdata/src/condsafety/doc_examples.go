package condsafety

import "sync"

//gohawk:example flagged Waiting without the mutex
func unlockedWait() {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	condition.Wait() // want "Cond.Wait called with its mutex unlocked"
}

//gohawk:example end

//gohawk:example ok
func lockedWait() {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	ready := false
	go func() {
		mutex.Lock()
		ready = true
		condition.Signal()
		mutex.Unlock()
	}()
	mutex.Lock()
	for !ready {
		condition.Wait()
	}
	mutex.Unlock()
}

//gohawk:example end
