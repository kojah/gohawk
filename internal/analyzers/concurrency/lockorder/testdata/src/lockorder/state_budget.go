package lockorder

import "sync"

type budgetPair struct {
	first, second sync.Mutex
}

// Budget exhaustion deliberately loses even an early recursive-acquire
// witness: incomplete all-return evidence must not publish partial decisions
// or let this function contribute an order edge to another function's cycle.
func stateBudgetUnknown(pair *budgetPair, a, b, c, d, e, f, g, h bool, noise func() bool) {
	mu, other := &pair.first, &pair.second
	mu.Lock()
	mu.Lock()
	other.Lock()
	if a {
		noise()
	}
	if b {
		noise()
	}
	if c {
		noise()
	}
	if d {
		noise()
	}
	if e {
		noise()
	}
	if f {
		noise()
	}
	if g {
		noise()
	}
	if h {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	if noise() {
		noise()
	}
	other.Unlock()
	mu.Unlock()
}

func stateBudgetOpposite(pair *budgetPair) {
	mu, other := &pair.first, &pair.second
	other.Lock()
	mu.Lock()
	mu.Unlock()
	other.Unlock()
}
