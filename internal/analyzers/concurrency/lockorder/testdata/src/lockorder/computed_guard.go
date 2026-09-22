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

// The incoming edge fixes this local flag even though its merged SSA value
// represents both constants. Only the path that retained the lock unlocks it.
func releasedOnIncomingEdge(mu *sync.Mutex, early bool) {
	mu.Lock()
	unlocked := false
	if early {
		mu.Unlock()
		unlocked = true
	}
	if !unlocked {
		mu.Unlock()
		return
	}
}

func wrongIncomingReleaseFlag(mu *sync.Mutex, early bool) {
	mu.Lock()
	unlocked := false
	if early {
		unlocked = true
	}
	if !unlocked {
		mu.Unlock()
		return
	}
	return // want "lock .* is not released on this return path"
}

// A value from a previous block is not a known constant just because one
// predecessor supplies a constant. The other path must remain reportable.
func unknownIncomingReleaseFlag(mu *sync.Mutex, early, skip bool) {
	mu.Lock()
	if early {
		mu.Unlock()
		skip = true
	}
	if !skip {
		mu.Unlock()
		return
	}
	return // want "lock .* is not released on this return path"
}
