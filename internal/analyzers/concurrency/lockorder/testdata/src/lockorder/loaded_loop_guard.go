package lockorder

import "sync"

// Changed mutable guards can really leak a lock. Without proving their values
// differ, repeated loaded guards remain unknown, not established cleanup.
type loadedLoopOwner struct {
	mu            sync.Mutex
	active, other bool
}

func (o *loadedLoopOwner) pairedLoadedGuard(items []int) {
	for range items {
		if o.active {
			o.mu.Lock()
		}
		if o.active {
			o.mu.Unlock()
		}
	}
}

func (o *loadedLoopOwner) duplicateInsideGuard(items []int) {
	for range items {
		if o.active {
			o.mu.Lock()
			o.mu.Lock() // want "lock .* is acquired while already held"
		}
		if o.active {
			o.mu.Unlock()
		}
	}
}

func (o *loadedLoopOwner) distinctLoadedGuard(items []int) {
	for range items {
		if o.active {
			o.mu.Lock() // want "lock .* is acquired while already held"
		}
		if o.other {
			o.mu.Unlock()
		}
	}
}

func (o *loadedLoopOwner) oppositeLoadedGuard(items []int) {
	for range items {
		if o.active {
			o.mu.Lock() // want "lock .* is acquired while already held"
		}
		if !o.active {
			o.mu.Unlock()
		}
	}
}
