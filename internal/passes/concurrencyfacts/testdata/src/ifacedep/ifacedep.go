package ifacedep

import "sync"

// Doer is implemented by the importing package.
type Doer interface{ Do() }

// WithLock calls d.Do while holding mu. Its published fact keeps the call as
// an interface hole.
func WithLock(mu *sync.Mutex, d Doer) {
	mu.Lock()
	d.Do()
	mu.Unlock()
}
