package lockorder

import "sync"

// The local flag is fixed at a merge, then used after unrelated branches.
func carriedReleaseState(mu *sync.Mutex, early, work, reserve bool) {
	mu.Lock()
	locked := true
	if early {
		mu.Unlock()
		locked = false
	}
	if work {
		if reserve {
			if !locked {
				mu.Lock()
			}
			mu.Unlock()
			locked = false
		}
	}
	if locked {
		mu.Unlock()
	}
}

func carriedWrongReleaseState(mu *sync.Mutex, early, work bool) {
	mu.Lock()
	locked := true
	if early {
		locked = false
	}
	if work {
		if locked {
			mu.Unlock()
			return
		}
	}
	if locked {
		mu.Unlock()
	}
	return // want "lock .* is not released on this return path"
}
