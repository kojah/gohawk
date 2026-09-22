package lockorder

import "sync"

func privateMutexReturn(fail bool) {
	var mu sync.Mutex
	mu.Lock()
	if fail {
		return
	}
	mu.Unlock()
}

func sharedMutexReturn(mu *sync.Mutex, fail bool) {
	mu.Lock()
	if fail {
		return // want "not released on this return path"
	}
	mu.Unlock()
}

func privateMutexStillDeadlocks() {
	var mu sync.Mutex
	mu.Lock()
	mu.Lock() // want "is acquired while already held"
	mu.Unlock()
}

func privateMutexPublished(out chan *sync.Mutex, fail bool) {
	mu := new(sync.Mutex)
	out <- mu
	mu.Lock()
	if fail {
		return // want "not released on this return path"
	}
	mu.Unlock()
}
