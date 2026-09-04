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

// An embedded mutex promotes RLock onto the struct, and the object it protects
// is still the struct.
type readEmbedded struct {
	sync.RWMutex
	routes map[string]int
}

func (e *readEmbedded) touch(k string) {
	e.RLock()
	defer e.RUnlock()
	e.routes[k] = 1 // want "write while only the read lock \\(\\*lockorder.readEmbedded\\).touch.e.RWMutex is held"
}

// The owner is whichever value holds the lock, which need not be the receiver.
type readInner struct {
	mu    sync.RWMutex
	count int
}

type readOuter struct{ in readInner }

func (o *readOuter) record() {
	o.in.mu.RLock()
	defer o.in.mu.RUnlock()
	o.in.count++ // want "write while only the read lock \\(\\*lockorder.readOuter\\).record.o.in.mu is held"
}

// A slice header loaded out of the owner shares the owner's backing array, so
// writing an element writes the owner. The index need not be constant.
type readSliced struct {
	mu      sync.RWMutex
	list    []int
	entries []*readEntry
}

func (s *readSliced) first() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.list[0] = 1 // want "write while only the read lock \\(\\*lockorder.readSliced\\).first.s.mu is held"
}

func (s *readSliced) at(index int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.list[index] = 1 // want "write while only the read lock \\(\\*lockorder.readSliced\\).at.s.mu is held"
}

// Accepted: an element pointer loaded out of the slice names a different
// object, so mutating what it points at is not a write to the owner. This is
// the same boundary the map case keeps, and it is why only the slice header
// itself is followed through its load.
func (s *readSliced) bumpEntry() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.entries) > 0 {
		s.entries[0].n++
	}
}

// A builtin that mutates its argument is a call rather than a store, so it
// reaches none of the store cases. Emptying a map or overwriting a slice under
// a read lock races exactly as assigning to it does.
type builtinCache struct {
	mu    sync.RWMutex
	items map[string]int
	buf   []byte
}

func (c *builtinCache) drop(k string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	delete(c.items, k) // want "write while only the read lock \\(\\*lockorder.builtinCache\\).drop.c.mu is held"
}

func (c *builtinCache) empty() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	clear(c.items) // want "write while only the read lock \\(\\*lockorder.builtinCache\\).empty.c.mu is held"
}

func (c *builtinCache) overwrite(src []byte) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	copy(c.buf, src) // want "write while only the read lock \\(\\*lockorder.builtinCache\\).overwrite.c.mu is held"
}

// copy takes its destination first, so reading the owner's slice into a
// caller's buffer is the read a read lock permits.
func (c *builtinCache) readInto(dst []byte) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	copy(dst, c.buf)
}

// A builtin mutating something other than the owner is not a write to it.
func (c *builtinCache) dropElsewhere(other map[string]int, k string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	delete(other, k)
}

// The write lock is the correct one to hold for a builtin that mutates.
func (c *builtinCache) dropLocked(k string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, k)
}
