package lockorder

import "sync"

// Missing-release does not correlate repeated loads of optional mutex fields.
// A nil guard can change through aliases, so this boundary remains unknown.
// Loaded Boolean guards are also unknown; actual mutation between acquisition
// and release is intentionally outside this narrowed missing-release proof.
type optionalOwner struct {
	mu    *sync.Mutex
	count int
}

type messageGuard struct{ locked bool }

func copiedMessageGuard(mu *sync.RWMutex, message messageGuard) {
	if !message.locked {
		mu.RLock()
	}
	if !message.locked {
		mu.RUnlock()
	}
}

func loadedGuardDoesNotHideUnguardedLock(mu *sync.Mutex, message messageGuard) {
	mu.Lock()
	if message.locked {
		mu.Unlock()
		return
	}
	return // want "not released on this return path"
}

func optionalOwnerUpdate(owner *optionalOwner) {
	if owner.mu != nil {
		owner.mu.Lock()
	}
	owner.count++
	if owner.mu != nil {
		owner.mu.Unlock()
	}
}

func optionalOwnerUnrelatedCondition(mu *sync.Mutex, acquire, release bool) {
	if acquire {
		mu.Lock()
	}
	if release {
		mu.Unlock()
	}
	return // want "lock .* is not released on this return path"
}

func optionalDirectPointerMissingRelease(mu *sync.Mutex, release bool) {
	if mu != nil {
		mu.Lock()
	}
	if mu != nil && release {
		mu.Unlock()
	}
	return // want "lock .* is not released on this return path"
}

func optionalOwnerReplaced(owner *optionalOwner, replace bool) {
	if owner.mu != nil {
		owner.mu.Lock()
		defer owner.mu.Unlock()
	}
	if replace {
		owner.mu = nil
	}
}

func changedBooleanGuard(mu *sync.Mutex, guard, next bool) {
	if guard {
		mu.Lock()
	}
	guard = next
	if guard {
		mu.Unlock()
	}
	return // want "lock .* is not released on this return path"
}

type unlockOwner struct{ mu sync.Mutex }
type unlockResult interface{ Unlock() }
type emptyUnlock struct{}

func (emptyUnlock) Unlock()        {}
func (owner *unlockOwner) Unlock() { owner.mu.Unlock() }

func (owner *unlockOwner) acquire(outcome int) unlockResult {
	owner.mu.Lock()
	if outcome == 0 {
		owner.mu.Unlock()
		return emptyUnlock{}
	}
	if outcome == 1 {
		return owner
	}
	owner.mu.Unlock()
	return nil
}

func (owner *unlockOwner) acquireWrongOwner(other *unlockOwner, outcome int) unlockResult {
	owner.mu.Lock()
	if outcome == 0 {
		owner.mu.Unlock()
		return emptyUnlock{}
	}
	if outcome == 1 {
		return other // want "lock .* is not released on this return path"
	}
	owner.mu.Unlock()
	return nil
}
