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

func dynamicallyCounted(n int) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	mu.Lock()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() { mu.Lock(); wg.Done(); mu.Unlock() }()
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
