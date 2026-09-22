package dependency

import "sync"

func Pair(a, b *sync.Mutex) { a.Lock(); b.Lock(); b.Unlock(); a.Unlock() }
func Conditional(a *sync.Mutex, yes bool) {
	if yes {
		a.Lock()
		a.Unlock()
	}
}
func Spawn(a *sync.Mutex) { go func() { a.Lock(); a.Unlock() }() }
func Empty(n int) int     { return n + 1 }
func Local()              { var mu sync.Mutex; mu.Lock(); mu.Unlock() }
func Opaque(a *sync.Mutex)
