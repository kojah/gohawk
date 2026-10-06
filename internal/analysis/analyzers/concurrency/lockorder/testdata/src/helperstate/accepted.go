package helperstate

import (
	"mutexstate"
	"sync"
)

// Complete effects preserve chronology, caller-owned contracts, and exact
// resources. Opaque/conditional calls do not prove a returned held state.
func balanced(mu *sync.Mutex)              { mutexstate.Balanced(mu); mu.Lock(); mu.Unlock() }
func released(mu *sync.Mutex)              { mutexstate.Acquire(mu); mutexstate.Release(mu); mu.Lock(); mu.Unlock() }
func different(a, b *sync.Mutex)           { mutexstate.Acquire(a); b.Lock(); b.Unlock(); a.Unlock() }
func deferredRelease(mu *sync.Mutex)       { mutexstate.Acquire(mu); defer mutexstate.Release(mu) }
func returnsHeld(mu *sync.Mutex)           { mutexstate.Acquire(mu) }
func conditional(mu *sync.Mutex, yes bool) { mutexstate.MaybeAcquire(mu, yes); mu.Lock(); mu.Unlock() }
func ambiguousRelease(mu *sync.Mutex, yes bool) {
	mu.Lock()
	mutexstate.MaybeRelease(mu, yes)
	mu.Lock()
	mu.Unlock()
}
func misleadingNames(mu *sync.Mutex) {
	mutexstate.RLock(mu)
	mutexstate.RUnlock(mu)
	mu.Lock()
	mu.Unlock()
}
func localReacquire(mu *sync.Mutex) { mu.Unlock(); mu.Lock() }
func reacquireThenRelease(mu *sync.Mutex) {
	mu.Lock()
	localReacquire(mu)
	mu.Unlock()
	mu.Lock()
	mu.Unlock()
}
func separateInstances() {
	for range 2 {
		mu := new(sync.Mutex)
		mutexstate.Acquire(mu)
		defer mutexstate.Release(mu)
	}
}
