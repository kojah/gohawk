package concurrentcapture

import "sync"

// These simple workers let the shared ordered lock region distinguish a
// genuine guard from an unrelated lock elsewhere in the closure.
func heldAtMutation(items []int) {
	var mu sync.Mutex
	var count int
	for range items {
		go func() {
			mu.Lock()
			count++
			mu.Unlock()
		}()
	}
}

func releasedBeforeMutation(items []int) {
	var mu sync.Mutex
	var count int
	for range items {
		go func() {
			mu.Lock()
			mu.Unlock()
			count++ // want "captured local count is mutated"
		}()
	}
}

func lockOnlyAfterMutation(items []int) {
	var mu sync.Mutex
	var count int
	for range items {
		go func() {
			count++ // want "captured local count is mutated"
			mu.Lock()
			mu.Unlock()
		}()
	}
}

func takeLock(mu *sync.Mutex) { mu.Lock() }

func helperHoldsAtMutation(items []int) {
	var mu sync.Mutex
	var count int
	for range items {
		go func() {
			takeLock(&mu)
			count++
			mu.Unlock()
		}()
	}
}
