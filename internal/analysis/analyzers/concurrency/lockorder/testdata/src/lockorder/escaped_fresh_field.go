package lockorder

import "sync"

// Fresh initialization followed by opaque publication leaves field identity
// uncertain, not proven fresh. An opaque publisher could replace the pointer;
// that deliberately remains a coverage loss rather than a safety claim.
type escapedFreshField struct {
	mu, unrelated *sync.Mutex
	stamp         int
}

var escapedFreshGuard sync.Mutex
var escapedFreshOwners []*escapedFreshField

func touchFreshOtherField(owner *escapedFreshField) { owner.stamp++ }

func escapedFreshConstruction(publish func(*escapedFreshField)) {
	escapedFreshGuard.Lock()
	defer escapedFreshGuard.Unlock()
	owner := &escapedFreshField{mu: new(sync.Mutex)}
	touchFreshOtherField(owner)
	publish(owner)
	escapedFreshOwners = append(escapedFreshOwners, owner)
	owner.mu.Lock()
	owner.mu.Unlock()
}

func escapedFreshReverse(owner *escapedFreshField) {
	owner.mu.Lock()
	escapedFreshGuard.Lock()
	escapedFreshGuard.Unlock()
	owner.mu.Unlock()
}

type replacedFreshField struct{ mu, unrelated *sync.Mutex }

var replacedFreshGuard sync.Mutex

func replaceFreshSlot(owner *replacedFreshField, shared *sync.Mutex) { owner.mu = shared }

func replacedFreshConstruction(shared *sync.Mutex, publish func(*replacedFreshField)) {
	replacedFreshGuard.Lock()
	defer replacedFreshGuard.Unlock()
	owner := &replacedFreshField{mu: new(sync.Mutex)}
	replaceFreshSlot(owner, shared)
	publish(owner)
	owner.mu.Lock()
	owner.mu.Unlock()
}

func replacedFreshReverse(owner *replacedFreshField) {
	owner.mu.Lock()
	replacedFreshGuard.Lock() // want "contradictory lock order: .*"
	replacedFreshGuard.Unlock()
	owner.mu.Unlock()
}

type sharedInitialField struct{ mu, unrelated *sync.Mutex }

var sharedInitialGuard sync.Mutex

func sharedInitialConstruction(shared *sync.Mutex, publish func(*sharedInitialField)) {
	sharedInitialGuard.Lock()
	defer sharedInitialGuard.Unlock()
	owner := &sharedInitialField{mu: shared, unrelated: new(sync.Mutex)}
	publish(owner)
	owner.mu.Lock()
	owner.mu.Unlock()
}

func sharedInitialReverse(owner *sharedInitialField) {
	owner.mu.Lock()
	sharedInitialGuard.Lock() // want "contradictory lock order: .*"
	sharedInitialGuard.Unlock()
	owner.mu.Unlock()
}

type slotReplacedField struct{ mu *sync.Mutex }

var slotReplacedGuard sync.Mutex

func replaceMutexSlot(slot **sync.Mutex, shared *sync.Mutex) { *slot = shared }

func slotReplacedConstruction(shared *sync.Mutex, publish func(*slotReplacedField)) {
	slotReplacedGuard.Lock()
	defer slotReplacedGuard.Unlock()
	owner := &slotReplacedField{mu: new(sync.Mutex)}
	replaceMutexSlot(&owner.mu, shared)
	publish(owner)
	owner.mu.Lock()
	owner.mu.Unlock()
}

func slotReplacedReverse(owner *slotReplacedField) {
	owner.mu.Lock()
	slotReplacedGuard.Lock() // want "contradictory lock order: .*"
	slotReplacedGuard.Unlock()
	owner.mu.Unlock()
}
