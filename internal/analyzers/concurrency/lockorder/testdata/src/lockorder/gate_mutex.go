package lockorder

import "sync"

// gateRegistry mirrors a package-install gate: the registry lock publishes a
// newly locked gate, and finishing the operation takes the registry lock while
// that gate is still held. The inversion is reported once even when the flow
// walk reaches the same relation through multiple states.
type gateRegistry struct {
	sync.Mutex
	gates map[string]*sync.Mutex
}

func (registry *gateRegistry) begin(key string) {
	registry.Lock()
	gate := new(sync.Mutex)
	registry.gates[key] = gate
	gate.Lock()
	registry.Unlock()
	registry.download(key) // want "contradictory lock order: .*gateRegistry.* and .*local:new:"
}

func (registry *gateRegistry) download(key string) {
	registry.finish(key)
}

func (registry *gateRegistry) finish(key string) {
	registry.Lock()
	defer registry.Unlock()
	gate := registry.gates[key]
	gate.Unlock()
}
