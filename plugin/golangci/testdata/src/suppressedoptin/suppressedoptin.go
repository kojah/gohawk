package suppressedoptin

import "sync"

// mismatched-release is experimental, so the default profile must not report it.
type cache struct {
	mu   sync.RWMutex
	hits int
}

func (c *cache) read() int {
	c.mu.RLock()
	defer c.mu.Unlock()
	return c.hits
}
