package ordercycles

import "sync"

var a, b, c sync.Mutex

func ab() { a.Lock(); defer a.Unlock(); b.Lock(); b.Unlock() }
func bc() { b.Lock(); defer b.Unlock(); c.Lock(); c.Unlock() }
func ca() {
	c.Lock()
	defer c.Unlock()
	a.Lock() // want "contradictory lock order: c -> a -> b -> c"
	a.Unlock()
}

// Separate instances of one field class must not create a self-cycle.
type peer struct{ mu sync.Mutex }

func peers(left, right *peer) {
	left.mu.Lock()
	right.mu.Lock()
	right.mu.Unlock()
	left.mu.Unlock()
}

var first, second, third sync.Mutex

// Sequential acquisition is not nesting, so it contributes no closing edge.
func sequential() {
	first.Lock()
	first.Unlock()
	second.Lock()
	second.Unlock()
}
func chain() {
	second.Lock()
	defer second.Unlock()
	third.Lock()
	third.Unlock()
}
func end() {
	third.Lock()
	defer third.Unlock()
	first.Lock()
	first.Unlock()
}

var readerA, readerB sync.RWMutex

// Read mode is evidence, not an exemption: queued writers can block readers.
func readAB() { readerA.RLock(); defer readerA.RUnlock(); readerB.RLock(); readerB.RUnlock() }
func readBA() {
	readerB.RLock()
	defer readerB.RUnlock()
	readerA.RLock() // want "contradictory lock order: readerA and readerB"
	readerA.RUnlock()
}

var outer, inner sync.RWMutex

func helper()        { leaf() }
func leaf()          { inner.RLock(); inner.RUnlock() }
func throughHelper() { outer.Lock(); defer outer.Unlock(); helper() }
func reverseHelper() {
	inner.RLock()
	defer inner.RUnlock()
	outer.Lock() // want "contradictory lock order: outer and inner"
	outer.Unlock()
}

// Asynchronous and deferred helper acquisitions do not run while held here.
var asyncA, asyncB sync.Mutex

func asynchronousLeaf() { asyncB.Lock(); asyncB.Unlock() }
func asynchronous()     { asyncA.Lock(); go asynchronousLeaf(); asyncA.Unlock() }
func deferred()         { asyncA.Lock(); defer asynchronousLeaf(); asyncA.Unlock() }
func reverseAsync()     { asyncB.Lock(); defer asyncB.Unlock(); asyncA.Lock(); asyncA.Unlock() }

// Distinct local allocations must not create a cycle across functions.
func localAB() { var x, y sync.Mutex; x.Lock(); y.Lock(); y.Unlock(); x.Unlock() }
func localBA() { var x, y sync.Mutex; y.Lock(); x.Lock(); x.Unlock(); y.Unlock() }

// A pooled receiver can be a different instance on every iteration. Its
// declaration class must not complete a longer cycle across unrelated owners.
type pooled struct{ mu sync.Mutex }
var queueA, queueB sync.Mutex
func pooledFirst(queue <-chan *pooled) {
	for item := range queue { item.mu.Lock(); queueA.Lock(); queueA.Unlock(); item.mu.Unlock() }
}
func pooledMiddle() { queueA.Lock(); defer queueA.Unlock(); queueB.Lock(); queueB.Unlock() }
func pooledLast(item *pooled) { queueB.Lock(); defer queueB.Unlock(); item.mu.Lock(); item.mu.Unlock() }

var redundantA, redundantB, redundantC sync.Mutex
func redundantAB() { redundantA.Lock(); defer redundantA.Unlock(); redundantB.Lock(); redundantB.Unlock() }
func redundantBA() {
	redundantB.Lock(); defer redundantB.Unlock()
	redundantA.Lock() // want "contradictory lock order: redundantA and redundantB"
	redundantA.Unlock()
}
func redundantBC() { redundantB.Lock(); defer redundantB.Unlock(); redundantC.Lock(); redundantC.Unlock() }
// A-B already has a shorter contradictory-order witness.
func redundantCA() { redundantC.Lock(); defer redundantC.Unlock(); redundantA.Lock(); redundantA.Unlock() }
