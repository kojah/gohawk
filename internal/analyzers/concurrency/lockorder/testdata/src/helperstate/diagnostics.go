package helperstate

import (
	"mutexstate"
	"sync"
)

func recursive(mu *sync.Mutex) {
	mutexstate.Acquire(mu)
	mu.Lock() // want "acquired while already held"
	mu.Unlock()
}
func helperRecursive(mu *sync.Mutex) {
	mu.Lock()
	mutexstate.Acquire(mu) // want "acquired while already held"
	mu.Unlock()
}
func restoredHeld(mu *sync.Mutex) {
	mu.Lock()
	mutexstate.Reacquire(mu)
	mu.Lock() // want "acquired while already held"
	mu.Unlock()
}
func missing(mu *sync.Mutex, early bool) {
	mutexstate.Acquire(mu)
	if early {
		return // want "not released on this return path"
	}
	mu.Unlock()
}
