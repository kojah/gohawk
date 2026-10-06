package lockorder

import "sync"

// Nested callbacks can make an unchanged captured owner cell opaque to the
// bounded storage query. A later Unlock may still release the original mutex.
type uncertainUnlockOwner struct {
	mu, other sync.Mutex
	n         int
}

func (owner *uncertainUnlockOwner) nestedCallbackRelease(register func(func()), fail bool) {
	owner.mu.Lock()
	if fail {
		owner.mu.Unlock()
		return
	}
	register(func() {
		register(func() { owner.n++ })
	})
	owner.mu.Unlock()
}

func (owner *uncertainUnlockOwner) nestedCallbackWrongField(register func(func()), fail bool) {
	owner.mu.Lock()
	if fail {
		owner.mu.Unlock()
		return
	}
	register(func() {
		register(func() { owner.n++ })
	})
	owner.other.Unlock()
	return // want "lock `owner\\.mu` is not released on this return path"
}
