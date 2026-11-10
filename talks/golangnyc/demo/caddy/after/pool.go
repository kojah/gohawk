package after

import "sync"

// Pool models one slot in Caddy's UsagePool. The map and reference counts
// are omitted so the constructor-failure lock inversion fits on screen.
type Pool struct {
	mu      sync.RWMutex
	item    entry
	present bool
}

type entry struct {
	mu    sync.RWMutex
	value any
	err   error
}

// Construct is called once per Pool; Range may run concurrently.
// This reduction omits replacement entries and reference counting.
func (p *Pool) Construct(build func() (any, error)) error {
	p.mu.Lock()
	p.item.mu.Lock()
	p.present = true
	p.mu.Unlock()

	value, err := build()
	p.item.value, p.item.err = value, err
	p.item.mu.Unlock() // Release the entry before waiting for the pool.
	if err != nil {
		p.mu.Lock()
		p.present = false
		p.mu.Unlock()
	}
	return err
}

func (p *Pool) Range(visit func(any)) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.present {
		return
	}
	p.item.mu.RLock() // Pool -> entry; cleanup no longer holds the entry lock.
	value, err := p.item.value, p.item.err
	p.item.mu.RUnlock()
	if err == nil {
		visit(value)
	}
}
