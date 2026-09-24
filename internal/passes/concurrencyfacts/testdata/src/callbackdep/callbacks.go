package callbackdep

import "sync"

// WithLock runs f while holding mu. Its published fact keeps f as a hole.
func WithLock(mu *sync.Mutex, f func()) {
	mu.Lock()
	f()
	mu.Unlock()
}
