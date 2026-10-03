package initialpublication

import "sync"

// Initial map publication may be initialization under the owner's writer.
// Caller/reader contracts remain unknown: this is not a safety guarantee.
// Known gap: a reader that bypasses that writer can make the first acquisition
// contend and create a real cycle. The old gate_mutex fixture covered that
// potential cycle; it is removed rather than retained as an accepted bug.
type registry struct {
	mu      sync.Mutex
	entries map[string]*sync.Mutex
}

func (r *registry) begin(key string) {
	r.mu.Lock()
	gate := new(sync.Mutex)
	r.entries[key] = gate
	gate.Lock()
	r.mu.Unlock()
	finish(&r.mu, gate)
}
func finish(owner, gate *sync.Mutex) { owner.Lock(); owner.Unlock(); gate.Unlock() }

// The held writer belongs to another object, so its edge stays visible.
type different struct{ entries map[string]*sync.Mutex }
type writer struct{ mu sync.Mutex }

func (r *different) begin(w *writer, key string) {
	w.mu.Lock()
	gate := new(sync.Mutex)
	r.entries[key] = gate
	gate.Lock()
	w.mu.Unlock()
	finish(&w.mu, gate) // want "contradictory lock order"
}

// The allocation was exposed to a call before publication.
type exposed struct {
	mu      sync.Mutex
	entries map[string]*sync.Mutex
}

var saved *sync.Mutex

func expose(gate *sync.Mutex) { saved = gate }
func (r *exposed) begin(key string) {
	r.mu.Lock()
	gate := new(sync.Mutex)
	expose(gate)
	r.entries[key] = gate
	gate.Lock()
	r.mu.Unlock()
	finish(&r.mu, gate) // want "contradictory lock order"
}
