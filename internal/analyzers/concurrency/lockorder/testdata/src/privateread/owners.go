package privateread

import "sync"

// Fresh local owners cannot be reached by another reader yet. Once exposed,
// the read-lock write question is retained; parameter ownership is uncertain.
type owner struct {
	mu    sync.RWMutex
	count int
}

var published *owner

func localOnly() {
	o := new(owner)
	o.mu.RLock()
	o.count++
	o.mu.RUnlock()
}

func initializeBeforePublication() {
	o := new(owner)
	o.mu.RLock()
	o.count++
	o.mu.RUnlock()
	published = o
}

func publishedBeforeWrite() {
	o := new(owner)
	published = o
	o.mu.RLock()
	o.count++ // want "write while only the read lock .* is held"
	o.mu.RUnlock()
}

func callerOwned(o *owner) {
	o.mu.RLock()
	o.count++ // want "write while only the read lock .* is held"
	o.mu.RUnlock()
}

// A fresh wrapper does not make its borrowed storage private.
type containers struct {
	mu     sync.RWMutex
	items  map[string]int
	values []int
}

func localContainers() {
	o := &containers{items: make(map[string]int), values: make([]int, 2)}
	o.mu.RLock()
	o.items["key"] = 1
	copy(o.values, []int{1, 2})
	o.values[0] = 3
	o.mu.RUnlock()
}

func borrowedMap(items map[string]int) {
	o := &containers{items: items}
	o.mu.RLock()
	o.items["key"] = 1 // want "write while only the read lock .* is held"
	clear(o.items)     // want "write while only the read lock .* is held"
	o.mu.RUnlock()
}

func borrowedSlice(values []int) {
	o := &containers{values: values}
	o.mu.RLock()
	copy(o.values, []int{1, 2}) // want "write while only the read lock .* is held"
	o.values[0] = 3             // want "write while only the read lock .* is held"
	o.mu.RUnlock()
}
