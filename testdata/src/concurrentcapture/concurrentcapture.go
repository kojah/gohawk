package concurrentcapture

import "sync"

func capturedError(items []int) error {
	var err error
	for range items {
		go func() {
			err = work() // want "captured local err is mutated by goroutines launched repeatedly"
		}()
	}
	return err
}

func capturedMap(items []int) map[int]bool {
	seen := map[int]bool{}
	for _, item := range items {
		go func() {
			seen[item] = true // want "captured local seen is mutated by goroutines launched repeatedly"
		}()
	}
	return seen
}

func closureLocal(items []int) {
	for range items {
		go func() {
			err := work()
			_ = err
		}()
	}
}

func guardedCapture(items []int) error {
	var mutex sync.Mutex
	var err error
	for range items {
		go func() {
			mutex.Lock()
			defer mutex.Unlock()
			err = work()
		}()
	}
	return err
}

func joinedEachIteration(items []int) error {
	var group sync.WaitGroup
	var err error
	for range items {
		group.Add(1)
		go func() {
			defer group.Done()
			err = work()
		}()
		group.Wait()
	}
	return err
}

func work() error { return nil }
