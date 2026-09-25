package suppressedoptin

import "sync"

// read-lock-write is experimental, so the default profile must not report it.
type cache struct {
	mu   sync.RWMutex
	hits int
}

func (c *cache) record() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	c.hits++
}
