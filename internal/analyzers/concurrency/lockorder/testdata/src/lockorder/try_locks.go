package lockorder

import "sync"

// Fixtures for lockorder/discarded-trylock.
//
// TryLock reports whether it acquired the lock. Discarding that answer leaves
// the code unable to tell a held lock from a contended one, so any guarded
// access that follows is unsynchronized and a later Unlock is a fatal unlock
// of an unlocked mutex.
//
// Known gap: only a call in statement position is judged. `defer mu.TryLock()`
// and `go mu.TryLock()` discard the result too, but they are rare enough that
// modelling them is not worth the extra surface.

type tryGuarded struct {
	mu    sync.Mutex
	rw    sync.RWMutex
	value int
}

// --- diagnostic forms ---

func (g *tryGuarded) discardsResult() {
	g.mu.TryLock() // want "TryLock result .* is discarded"
	g.value++
	g.mu.Unlock()
}

func (g *tryGuarded) discardsResultExplicitly() {
	_ = g.mu.TryLock() // want "TryLock result .* is discarded"
	g.value++
	g.mu.Unlock()
}

func (g *tryGuarded) discardsReadResult() {
	g.rw.TryRLock() // want "TryRLock result .* is discarded"
	_ = g.value
	g.rw.RUnlock()
}

func (g *tryGuarded) discardsWriteResultOnRWMutex() {
	g.rw.TryLock() // want "TryLock result .* is discarded"
	g.value++
	g.rw.Unlock()
}

// --- accepted: the result decides control flow ---

func (g *tryGuarded) branchesOnResult() {
	if g.mu.TryLock() {
		defer g.mu.Unlock()
		g.value++
	}
}

func (g *tryGuarded) branchesOnNegatedResult() bool {
	if !g.mu.TryLock() {
		return false
	}
	defer g.mu.Unlock()
	g.value++
	return true
}

func (g *tryGuarded) storesResultThenTests() {
	acquired := g.mu.TryLock()
	if !acquired {
		return
	}
	defer g.mu.Unlock()
	g.value++
}

func (g *tryGuarded) spinsUntilAcquired() {
	for !g.mu.TryLock() {
	}
	defer g.mu.Unlock()
	g.value++
}

// --- accepted: the result leaves this function, so the caller decides ---

func (g *tryGuarded) returnsResult() bool {
	return g.mu.TryLock()
}

func (g *tryGuarded) passesResultToHelper() {
	recordAcquisition(g.mu.TryLock())
}

func (g *tryGuarded) storesResultInField(state *acquisitionState) {
	state.acquired = g.rw.TryLock()
}

type acquisitionState struct{ acquired bool }

func recordAcquisition(bool) {}

// --- accepted: a misleading name on a type that is not a sync mutex ---

// throttle has a TryLock method that reports whether a slot was free, but it
// is not a sync mutex and carries no unlock obligation, so discarding the
// answer is ordinary use rather than a lost acquisition.
type throttle struct{ slots int }

func (t *throttle) TryLock() bool {
	if t.slots == 0 {
		return false
	}
	t.slots--
	return true
}

func discardsUnrelatedTryLock(t *throttle) {
	t.TryLock()
}

// gate embeds a mutex but shadows TryLock with its own always-succeeding
// method, so the call resolves to the wrapper rather than to sync.Mutex.
type gate struct{ mu sync.Mutex }

func (g *gate) TryLock() bool {
	g.mu.Lock()
	return true
}

func discardsWrapperTryLock(g *gate) {
	g.TryLock()
	g.mu.Unlock()
}
