package mutexstate

import "sync"

func Acquire(mu *sync.Mutex)   { mu.Lock() }
func Release(mu *sync.Mutex)   { mu.Unlock() }
func Balanced(mu *sync.Mutex)  { mu.Lock(); defer mu.Unlock() }
func Reacquire(mu *sync.Mutex) { mu.Unlock(); mu.Lock() }
func MaybeAcquire(mu *sync.Mutex, yes bool) {
	if yes {
		mu.Lock()
	}
}
func MaybeRelease(mu *sync.Mutex, yes bool) {
	if yes {
		mu.Unlock()
	}
}
func RLock(mu *sync.Mutex)   { mu.Lock() }
func RUnlock(mu *sync.Mutex) { mu.Unlock() }
