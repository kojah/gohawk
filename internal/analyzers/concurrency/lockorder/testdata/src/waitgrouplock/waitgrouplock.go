package waitgrouplock

import "sync"

// The accepted cases pin the exact counter, participant, and ordering
// requirements before the cycle proof is allowed to report.
func workerFinishesBeforeLock() {
	var mu sync.Mutex
	var group sync.WaitGroup
	group.Add(1)
	mu.Lock()
	go func() {
		group.Done()
		mu.Lock()
		mu.Unlock()
	}()
	group.Wait()
	mu.Unlock()
}

func parentReleasesBeforeWait() {
	var mu sync.Mutex
	var group sync.WaitGroup
	group.Add(1)
	mu.Lock()
	go func() {
		mu.Lock()
		group.Done()
		mu.Unlock()
	}()
	mu.Unlock()
	group.Wait()
}

func childMayFinishBeforeParentLocks() {
	var mu sync.Mutex
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		mu.Lock()
		group.Done()
		mu.Unlock()
	}()
	mu.Lock()
	group.Wait()
	mu.Unlock()
}

func otherWorkerCanCompleteCounter() {
	var mu sync.Mutex
	var group sync.WaitGroup
	group.Add(1)
	mu.Lock()
	go func() {
		mu.Lock()
		group.Done()
		mu.Unlock()
	}()
	go func() { group.Done() }()
	group.Wait()
	mu.Unlock()
}

func differentMutex() {
	var held, worker sync.Mutex
	var group sync.WaitGroup
	group.Add(1)
	held.Lock()
	go func() {
		worker.Lock()
		group.Done()
		worker.Unlock()
	}()
	group.Wait()
	held.Unlock()
}

func unknownCount() {
	var mu sync.Mutex
	var group sync.WaitGroup
	group.Add(2)
	mu.Lock()
	go func() {
		mu.Lock()
		group.Done()
		mu.Unlock()
	}()
	group.Wait()
	mu.Unlock()
}

func oneBlockedWorker() {
	var mu sync.Mutex
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

func twoBlockedWorkers() {
	var mu sync.Mutex
	var group sync.WaitGroup
	group.Add(1)
	group.Add(1)
	mu.Lock()
	go func() {
		mu.Lock()
		group.Done()
		mu.Unlock()
	}()
	go func() {
		mu.Lock()
		group.Done()
		mu.Unlock()
	}()
	group.Wait() // want "waits for counted workers that need the held lock"
	mu.Unlock()
}
