package lockorder

import "sync"

func computedGuard(mu *sync.Mutex, a, b bool) {
	guard := a && b
	if guard {
		mu.Lock()
	}
	if guard {
		mu.Unlock()
	}
}

func computedGuardMissingRelease(mu *sync.Mutex, a, b, release bool) {
	guard := a && b
	if guard {
		mu.Lock()
	}
	if release {
		mu.Unlock()
	}
	return // want "lock .* is not released on this return path"
}
