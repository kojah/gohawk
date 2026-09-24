package waitgrouplock

import "sync"

// The held mutex may belong to the caller; the group must still be fresh so
// no outside participant can decrement it.
func callerMutex(mu *sync.Mutex) {
	var group sync.WaitGroup
	group.Add(1)
	mu.Lock()
	go func() {
		mu.Lock()
		group.Done()
		mu.Unlock()
	}()
	group.Wait() // want "waits for counted workers that need the held lock"
	mu.Unlock()
}

func callerGroup(mu *sync.Mutex, group *sync.WaitGroup) {
	group.Add(1)
	mu.Lock()
	go func() {
		mu.Lock()
		group.Done()
		mu.Unlock()
	}()
	group.Wait()
	mu.Unlock()
}
