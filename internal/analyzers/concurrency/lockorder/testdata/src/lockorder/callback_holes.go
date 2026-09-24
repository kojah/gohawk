package lockorder

import "sync"

// Concurrency summaries keep a call through a function input as a hole and
// fill it at call sites that pass a known closure, so a helper that runs its
// callback under a lock composes with that callback's own acquisitions.

func holdWhileCalling(mu *sync.Mutex, f func()) {
	mu.Lock()
	f()
	mu.Unlock()
}

func callAfterRelease(mu *sync.Mutex, f func()) {
	mu.Lock()
	mu.Unlock()
	f()
}

func callbackRelocks() {
	var mu sync.Mutex
	holdWhileCalling(&mu, func() { // want "lock .*mu.* is acquired while already held"
		mu.Lock()
		mu.Unlock()
	})
}

// The callback takes a different mutex, which only nests.
func callbackOtherLock() {
	var mu, other sync.Mutex
	holdWhileCalling(&mu, func() {
		other.Lock()
		other.Unlock()
	})
}

// The helper releases before it calls back.
func callbackAfterRelease() {
	var mu sync.Mutex
	callAfterRelease(&mu, func() {
		mu.Lock()
		mu.Unlock()
	})
}

// The callback releases the held lock before taking it again.
func callbackReleasesFirst() {
	var mu sync.Mutex
	holdWhileCalling(&mu, func() {
		mu.Unlock()
		mu.Lock()
	})
}

// A forwarded callback is only known to this function's own callers.
func forwardedCallback(mu *sync.Mutex, f func()) {
	holdWhileCalling(mu, f)
}

// The closure reaches the mutex through a captured receiver, which this slice
// of callback binding does not resolve, so the acquisition stays unknown.
type callbackOwner struct{ mu sync.Mutex }

func (o *callbackOwner) locked(f func()) {
	o.mu.Lock()
	defer o.mu.Unlock()
	f()
}

func (o *callbackOwner) Relock() {
	o.locked(func() {
		o.mu.Lock()
		o.mu.Unlock()
	})
}
