package goroutineownership

import "sync"

// An embedded group selected from an unreadable owner may belong to a registry.
// No callback contract is inferred. Local and visible fresh owners still owe
// their join; reading an unrelated opaque owner cannot discharge that duty.
// Known gap: an opaque factory that actually returns a fresh owner is also
// unknown, as is a merge between a fresh owner and a registry-owned one.
type registryWorkerOwner struct{ group sync.WaitGroup }

type registryWorkerEnvelope struct{ worker registryWorkerOwner }

func opaqueRegistryOwner() any

func embeddedRegistryWorker(task func()) {
	owner := opaqueRegistryOwner().(*registryWorkerOwner)
	owner.group.Add(1)
	go func(group *sync.WaitGroup) {
		defer group.Done()
		task()
	}(&owner.group)
}

func capturedRegistryWorker(task func()) {
	owner, ok := opaqueRegistryOwner().(*registryWorkerOwner)
	if !ok {
		return
	}
	owner.group.Add(1)
	go func() {
		defer owner.group.Done()
		task()
	}()
}

func nestedRegistryWorker(task func()) {
	owner := opaqueRegistryOwner().(*registryWorkerEnvelope)
	owner.worker.group.Add(1)
	go func(group *sync.WaitGroup) {
		defer group.Done()
		task()
	}(&owner.worker.group)
}

func mixedRegistryWorker(task func(), registered bool) {
	owner := &registryWorkerOwner{}
	if registered {
		owner = opaqueRegistryOwner().(*registryWorkerOwner)
	}
	owner.group.Add(1)
	go func(group *sync.WaitGroup) {
		defer group.Done()
		task()
	}(&owner.group)
}

func freshEmbeddedWorker(task func()) {
	owner := &registryWorkerOwner{}
	owner.group.Add(1)
	go func(group *sync.WaitGroup) { // want "goroutine is not joined on every return path"
		defer group.Done()
		task()
	}(&owner.group)
}

func newVisibleWorkerOwner() *registryWorkerOwner { return &registryWorkerOwner{} }

func visibleFreshEmbeddedWorker(task func()) {
	owner := newVisibleWorkerOwner()
	owner.group.Add(1)
	go func(group *sync.WaitGroup) { // want "goroutine is not joined on every return path"
		defer group.Done()
		task()
	}(&owner.group)
}

func unrelatedRegistryOwner(task func()) {
	owner := &registryWorkerOwner{}
	_ = opaqueRegistryOwner()
	owner.group.Add(1)
	go func(group *sync.WaitGroup) { // want "goroutine is not joined on every return path"
		defer group.Done()
		task()
	}(&owner.group)
}

type pointerRegistryWorkerOwner struct{ group *sync.WaitGroup }

func replacedRegistryGroup(task func()) {
	owner := opaqueRegistryOwner().(*pointerRegistryWorkerOwner)
	owner.group = new(sync.WaitGroup)
	owner.group.Add(1)
	go func(group *sync.WaitGroup) { // want "goroutine is not joined on every return path"
		defer group.Done()
		task()
	}(owner.group)
}
