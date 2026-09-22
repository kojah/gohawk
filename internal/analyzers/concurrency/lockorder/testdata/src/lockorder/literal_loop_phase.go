package lockorder

import "sync"

// An exact initial phase survives retries while the initial lock is held.
// Unknown phase updates forget the literal; no iteration count is inferred.
func literalLoopPhase(mu *sync.Mutex, states <-chan int) {
	mu.Lock()
	phase := -1
	for {
		next := <-states & 255
		if next == phase {
			continue
		}
		if phase != -1 {
			mu.Lock()
		}
		mu.Unlock()
		phase = next
	}
}

func literalLoopPhaseMissingRelease(mu *sync.Mutex, states <-chan int) {
	mu.Lock()
	phase := -1
	for {
		next := <-states & 255
		if next == phase {
			continue
		}
		if phase != -1 {
			mu.Lock() // want "lock .* is acquired while already held"
		}
		phase = next
	}
}
