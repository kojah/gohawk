package lockjoin

import "sync"

func countedWaitGroup() {
	var mu sync.Mutex
	var wg sync.WaitGroup
	mu.Lock()
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { mu.Lock(); wg.Done(); mu.Unlock() }()
	}
	wg.Wait() // want "waits for counted workers that need the held lock"
	mu.Unlock()
}

func countedReleasedBeforeWait() {
	var mu sync.Mutex
	var wg sync.WaitGroup
	mu.Lock()
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { mu.Lock(); wg.Done(); mu.Unlock() }()
	}
	mu.Unlock()
	wg.Wait()
}

// A worker pool of unknown size deadlocks whenever it launches at least one
// worker: every copy needs the lock the parent holds across Wait, and another
// copy cannot release it. One representative worker proves that.
func dynamicallyCounted(n int) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	mu.Lock()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() { mu.Lock(); wg.Done(); mu.Unlock() }()
	}
	wg.Wait() // want "waits for counted workers that need the held lock"
	mu.Unlock()
}

func dynamicReleasedBeforeWait(items []int) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	mu.Lock()
	for range items {
		wg.Add(1)
		go func() { mu.Lock(); wg.Done(); mu.Unlock() }()
	}
	mu.Unlock()
	wg.Wait()
}

func dynamicWithoutTheLock(items []int) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	mu.Lock()
	for range items {
		wg.Add(1)
		go func() { wg.Done() }()
	}
	wg.Wait()
	mu.Unlock()
}

// An exact repetition count cannot repair registration after the launch or
// establish that a worker launched before Lock still needs to acquire it.
func countedAfterLaunch() {
	var mu sync.Mutex
	var wg sync.WaitGroup
	mu.Lock()
	for i := 0; i < 2; i++ {
		go func() { mu.Lock(); wg.Done(); mu.Unlock() }()
		wg.Add(1)
	}
	wg.Wait()
	mu.Unlock()
}

func countedBeforeLock() {
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { mu.Lock(); wg.Done(); mu.Unlock() }()
	}
	mu.Lock()
	wg.Wait()
	mu.Unlock()
}
