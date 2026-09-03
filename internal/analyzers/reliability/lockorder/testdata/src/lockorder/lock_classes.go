package lockorder

import "sync"

// Lock classes: contradictory-order compares the declaration a mutex lives in
// when its instance identity is confined to one function. Package variables
// keep instance identity and are covered in lockorder.go.
//
// Known gap: a mutex selected from a map or slice is still declined before it
// reaches this comparison, because that receiver may be a different lock on
// every iteration. cockroach#7504 reaches its lease that way in the original,
// so the classCache pair below passes the lease directly instead.

type classPair struct {
	first  sync.Mutex
	second sync.Mutex
}

// Two methods on one type invert the order of two fields of that type. Neither
// receiver can be resolved to the other's object, so only the field classes
// make the contradiction visible.
func (p *classPair) forward() {
	p.first.Lock()
	defer p.first.Unlock()
	p.second.Lock()
	defer p.second.Unlock()
}

func (p *classPair) reverse() {
	p.second.Lock()
	defer p.second.Unlock()
	p.first.Lock() // want "contradictory lock order: \\*lockorder.classPair.first and \\*lockorder.classPair.second"
	defer p.first.Unlock()
}

// A class cycle can cross types. cockroach#7504 deadlocked between a lease
// mutex and a name-cache mutex held in that order on one path and the reverse
// on another:
// https://github.com/cockroachdb/cockroach/pull/7504
type classLease struct {
	mu sync.Mutex
}

type classCache struct {
	mu sync.Mutex
}

func (c *classCache) evict(lease *classLease) {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
}

func (c *classCache) lookup(lease *classLease) {
	c.mu.Lock()
	defer c.mu.Unlock()
	lease.mu.Lock() // want "contradictory lock order: \\*lockorder.classLease.mu and \\*lockorder.classCache.mu"
	defer lease.mu.Unlock()
}

// Accepted: two peers of one class. Whether these deadlock depends on which
// objects each caller passes, and no evidence here settles that, so ordering
// two instances of the same class proves nothing. This is the ordinary
// two-object update, and reporting it would flag every such routine.
type classAccount struct {
	mu      sync.Mutex
	balance int
}

func classTransfer(from, to *classAccount, amount int) {
	from.mu.Lock()
	defer from.mu.Unlock()
	to.mu.Lock()
	defer to.mu.Unlock()
	from.balance -= amount
	to.balance += amount
}

func classSettle(left, right *classAccount) {
	right.mu.Lock()
	defer right.mu.Unlock()
	left.mu.Lock()
	defer left.mu.Unlock()
}

// Accepted: two classes always taken in the same order.
type classIndex struct {
	mu sync.Mutex
}

type classStore struct {
	mu    sync.Mutex
	index *classIndex
}

func (s *classStore) insert() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.index.mu.Lock()
	defer s.index.mu.Unlock()
}

func (s *classStore) remove() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.index.mu.Lock()
	defer s.index.mu.Unlock()
}

// Accepted: locals declare no class another function could name, so they keep
// instance identity and never compare across functions.
func classLocalForward() {
	var first, second sync.Mutex
	first.Lock()
	defer first.Unlock()
	second.Lock()
	defer second.Unlock()
}

func classLocalReverse() {
	var first, second sync.Mutex
	second.Lock()
	defer second.Unlock()
	first.Lock()
	defer first.Unlock()
}
