package lockorder

import "sync"

// Loaded getter guards inherit direct-load uncertainty. Mutating the field
// between the checks remains a deliberate coverage gap, not a proved release.
type getterGuard struct{ enabled bool }

func (guard *getterGuard) Enabled() bool  { return guard.enabled }
func (*getterGuard) Always() bool         { return true }
func (guard *getterGuard) Inverted() bool { return !guard.enabled }

func loadedGetterBalanced(mu *sync.Mutex, guard *getterGuard, work func()) {
	if guard.Enabled() {
		mu.Lock()
	}
	work()
	if guard.Enabled() {
		mu.Unlock()
	}
}

func loadedGetterDoesNotHideUnguardedLock(mu *sync.Mutex, guard *getterGuard) {
	mu.Lock()
	if guard.Enabled() {
		mu.Unlock()
		return
	}
	return // want "not released on this return path"
}

func constantGetterMissingRelease(mu *sync.Mutex, guard *getterGuard, release bool) {
	if guard.Always() {
		mu.Lock()
	}
	if release {
		mu.Unlock()
	}
	return // want "not released on this return path"
}

func computedGetterMissingRelease(mu *sync.Mutex, guard *getterGuard, release bool) {
	if guard.Inverted() {
		mu.Lock()
	}
	if release {
		mu.Unlock()
	}
	return // want "not released on this return path"
}
