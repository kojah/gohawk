package lockorder

import "sync"

// Helpers retain an embedded mutex's exact caller root even when the caller
// never computes its field address. Conditional effects deliberately exercise
// the acquisition-witness summary, not a complete ordered-effects summary.
type helperFieldOwner struct{ mu sync.Mutex }

var freshCallerGuard sync.Mutex

func conditionalFieldLock(owner *helperFieldOwner, flag bool) {
	if flag {
		owner.mu.Lock()
		owner.mu.Unlock()
	}
}

func wrappedFieldLock(owner *helperFieldOwner, flag bool) { conditionalFieldLock(owner, flag) }

func constructWithFieldHelper(flag bool) *helperFieldOwner {
	freshCallerGuard.Lock()
	defer freshCallerGuard.Unlock()
	owner := new(helperFieldOwner)
	wrappedFieldLock(owner, flag)
	return owner
}

func establishedFieldOwner(owner *helperFieldOwner) {
	owner.mu.Lock()
	freshCallerGuard.Lock()
	freshCallerGuard.Unlock()
	owner.mu.Unlock()
}

var mixedCallerGuard sync.Mutex

func lockTwoFieldOwners(first, second *helperFieldOwner, flag bool) {
	conditionalFieldLock(first, flag)
	conditionalFieldLock(second, flag)
}

func mixedFreshAndSharedFieldOwner(shared *helperFieldOwner, flag bool) {
	mixedCallerGuard.Lock()
	defer mixedCallerGuard.Unlock()
	lockTwoFieldOwners(new(helperFieldOwner), shared, flag)
}

func establishedMixedFieldOwner(owner *helperFieldOwner) {
	owner.mu.Lock()
	mixedCallerGuard.Lock() // want "contradictory lock order: .*"
	mixedCallerGuard.Unlock()
	owner.mu.Unlock()
}

func sameCallerFieldInversion(reverse, flag bool) {
	owner := new(helperFieldOwner)
	var other sync.Mutex
	if reverse {
		owner.mu.Lock()
		other.Lock()
		other.Unlock()
		owner.mu.Unlock()
		return
	}
	other.Lock()
	wrappedFieldLock(owner, flag) // want "contradictory lock order: .*"
	other.Unlock()
}
