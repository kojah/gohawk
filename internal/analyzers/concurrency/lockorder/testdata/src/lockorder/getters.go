package lockorder

import (
	"sync"

	"getterdependency"
)

type getterOwner struct {
	mu    sync.RWMutex
	other sync.RWMutex
}

func (owner *getterOwner) Mu() *sync.RWMutex    { return &owner.mu }
func (owner *getterOwner) Other() *sync.RWMutex { return &owner.other }

func getterBalanced(owner *getterOwner) {
	for range 3 {
		owner.Mu().RLock()
		owner.Mu().RUnlock()
	}
	owner.Mu().Lock()
	owner.mu.Unlock()
}

func getterDistinctOwners(left, right *getterOwner) {
	left.Mu().Lock()
	right.Mu().Lock()
	right.Mu().Unlock()
	left.Mu().Unlock()
}

func getterDistinctFields(owner *getterOwner) {
	owner.Mu().Lock()
	owner.Other().Lock()
	owner.Other().Unlock()
	owner.Mu().Unlock()
}

func getterRecursive(owner *getterOwner) {
	owner.Mu().Lock()
	owner.Mu().Lock() // want "is acquired while already held"
	owner.Mu().Unlock()
}

func getterMissingRelease(owner *getterOwner, fail bool) {
	owner.Mu().Lock()
	if fail {
		return // want "not released on this return path"
	}
	owner.Mu().Unlock()
}

func importedGetter(owner *getterdependency.Owner) {
	for range 3 {
		owner.Mu().Lock()
		owner.Mu().Unlock()
	}
}

type freshGetter struct{}

func (*freshGetter) Mu() *sync.Mutex { return new(sync.Mutex) }

func freshMutexes(owner *freshGetter) {
	left, right := owner.Mu(), owner.Mu()
	left.Lock()
	right.Lock()
	right.Unlock()
	left.Unlock()
}

// A selecting getter is deliberately opaque, not a promise to return one field.
func (owner *getterOwner) Select(which bool) *sync.RWMutex {
	if which {
		return &owner.mu
	}
	return &owner.other
}

func opaqueGetter(owner *getterOwner) {
	for range 3 {
		owner.Select(true).Lock()
		owner.Select(true).Unlock()
	}
}

func newGetterOwner() *getterOwner { return &getterOwner{} }

// A field below an opaque call result must not acquire a type-wide instance
// identity. Each call here really does allocate a different owner.
func fieldsFromUnknownOwner() {
	for range 3 {
		owner := newGetterOwner()
		owner.mu.Lock()
	}
}
