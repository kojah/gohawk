package lockorder

import "sync"

// Capturing an unchanged owner must not split one mutex's identity between
// the operations before and after the callback handoff.
type handoffOwner struct {
	mu sync.Mutex
	n  int
}

func (owner *handoffOwner) handoffUnchanged(register func(func()), fail bool) {
	owner.mu.Lock()
	if fail {
		owner.mu.Unlock()
		return
	}
	register(func() { owner.n++ })
	owner.mu.Unlock()
}

func (owner *handoffOwner) missingAfterHandoff(register func(func()), fail bool) {
	owner.mu.Lock()
	if fail {
		owner.mu.Unlock()
		return
	}
	register(func() { owner.n++ })
	return // want "lock `owner\\.mu` is not released on this return path"
}

func (owner *handoffOwner) deferredAroundRelock(work func(), fail bool) {
	owner.mu.Lock()
	if fail {
		owner.mu.Unlock()
		return
	}
	defer func() {
		owner.n = 0
		owner.mu.Unlock()
	}()
	for owner.n > 0 {
		owner.mu.Unlock()
		func() {
			defer owner.mu.Lock()
			work()
		}()
	}
}
