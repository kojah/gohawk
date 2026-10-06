package readlockfields

import "sync"

// A field-only receiver method carries no synchronization in its own frame.
// Callers may own it or supply a guard; neither contract is known here. That
// uncertainty is specific to the field, rather than to every field of its owner.
// Known gap: a setter called exclusively under the same mutex can hide a real
// read-lock write to its field. This check does not infer caller-supplied guards.
type cursor struct {
	mu      sync.RWMutex
	head    *int
	current *int
	count   int
}

func (c *cursor) reset() { c.head = nil; c.current = nil }
func (c *cursor) prepare() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	c.head = new(int)
	c.current = c.head
	c.count++ // want "write while only the read lock"
}

// An update of the old value is still not a field-to-guard contract.
type independent struct {
	mu     sync.RWMutex
	cursor int
	shared int
}

func (c *independent) reset() { c.cursor = 0 }
func (c *independent) advance() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	c.cursor++
	c.shared++ // want "write while only the read lock"
}

// A receiver method that calls other code is not the closed field-only body.
// A write-locked reset preserves the ordinary read-lock diagnostic.
type guarded struct {
	mu   sync.RWMutex
	head *int
}

func (c *guarded) reset() { c.mu.Lock(); c.head = nil; c.mu.Unlock() }
func (c *guarded) prepare() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	c.head = new(int) // want "write while only the read lock"
}

// The same field spelling on another declared type is unrelated evidence.
type distinct struct {
	mu   sync.RWMutex
	head *int
}

func (c *distinct) prepare() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	c.head = new(int) // want "write while only the read lock"
}
