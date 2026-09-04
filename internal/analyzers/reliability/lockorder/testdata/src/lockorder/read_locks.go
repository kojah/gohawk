package lockorder

import (
	"sync"
	"sync/atomic"
)

// Read-lock writes: a shared lock grants read access only, so mutating the
// object it protects races with every other reader.
//
// The claim is about the object, not about which field the lock guards, so a
// struct with more than one guard domain is left alone rather than guessed at.

type readEntry struct{ n int }

type readCache struct {
	mu     sync.RWMutex
	routes map[string]*readEntry
	hits   int
}

func (c *readCache) touch(k string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	c.routes[k] = &readEntry{} // want "write while only the read lock \\(\\*lockorder.readCache\\).touch.c.mu is held"
}

func (c *readCache) record() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	c.hits++ // want "write while only the read lock \\(\\*lockorder.readCache\\).record.c.mu is held"
}

// Accepted: the write lock grants write access.
func (c *readCache) store(k string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.routes[k] = &readEntry{}
}

// Accepted: an entry loaded out of the guarded map is a different cell, so
// mutating it is not a write to the object the lock protects.
func (c *readCache) bump(k string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if entry := c.routes[k]; entry != nil {
		entry.n++
	}
}

// Accepted: the read lock is released before the write.
func (c *readCache) upgrade(k string) {
	c.mu.RLock()
	_, present := c.routes[k]
	c.mu.RUnlock()
	if present {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.routes[k] = &readEntry{}
}

// Accepted: a local is not shared, whatever is held while it is written.
func (c *readCache) size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	local := readEntry{}
	local.n = len(c.routes)
	return local.n
}

type readDomains struct {
	mu      sync.RWMutex // guards routes
	routes  map[string]*readEntry
	metrics atomic.Int64 // guarded by atomics, not by mu
	other   sync.Mutex   // an independent guard domain
	counted int          // guarded by other
}

// Accepted: an atomic update is a call, not a store, and such a field is
// protected by atomics rather than by the lock.
func (d *readDomains) observe() {
	d.mu.RLock()
	defer d.mu.RUnlock()
	d.metrics.Add(1)
}

// Accepted: counted belongs to a second guard domain and is written under that
// domain's write lock. Deciding which mutex covers which field is guard
// inference this check does not do, so a write lock held on the same object
// leaves the write unproven rather than reportable.
func (d *readDomains) tally() {
	d.mu.RLock()
	defer d.mu.RUnlock()
	d.other.Lock()
	defer d.other.Unlock()
	d.counted++
}
