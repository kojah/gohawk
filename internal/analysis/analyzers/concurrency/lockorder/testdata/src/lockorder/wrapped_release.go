package lockorder

import "sync"

// Releases performed by a callee, and what the caller may claim afterwards.
//
// A release proven before every normal return transfers the obligation. A
// release the callee performs on only some of its paths settles nothing: the
// caller can no longer claim the lock is held, so neither an unreleased return
// nor a later acquisition may be reported against it.

// guardedMutex wraps a mutex and returns early on a nil receiver, so its
// Unlock releases on some paths but not on all of them.
type guardedMutex struct {
	sync.Mutex
	name string
}

func (m *guardedMutex) Unlock() {
	if m == nil {
		return
	}
	m.Mutex.Unlock()
}

type guardedCache struct {
	mu    guardedMutex
	items map[string]int
}

// Accepted: the pair is balanced inside the loop body, so no iteration begins
// with the lock held. The wrapper's release cannot be proven on every path,
// which must not turn the next iteration into a recursive acquisition.
func (c *guardedCache) readEach(keys []string) []int {
	var out []int
	for _, key := range keys {
		c.mu.Lock()
		value := c.items[key]
		c.mu.Unlock()
		out = append(out, value)
	}
	return out
}

// Accepted: the same unprovable release also stops this return from claiming
// the lock is still held.
func (c *guardedCache) readOne(key string) int {
	c.mu.Lock()
	value := c.items[key]
	c.mu.Unlock()
	return value
}
