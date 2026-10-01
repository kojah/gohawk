package goroutineownership

import "fieldworker"

type opaqueWorkerFields struct {
	worker *fieldworker.Worker
	other  *fieldworker.Worker
}

func cleanupOpaqueWorkerField(owner *opaqueWorkerFields, timeout <-chan struct{}) {
	done := make(chan struct{})
	go func() {
		owner.worker.Run()
		close(done)
	}()
	select {
	case <-done:
	case <-timeout:
		owner.worker.Stop()
	}
}

func cleanupOtherWorkerField(owner *opaqueWorkerFields, timeout <-chan struct{}) {
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		owner.worker.Run()
		close(done)
	}()
	select {
	case <-done:
	case <-timeout:
		owner.other.Stop()
	}
}

func cleanupOtherWorkerOwner(owner, other *opaqueWorkerFields, timeout <-chan struct{}) {
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		owner.worker.Run()
		close(done)
	}()
	select {
	case <-done:
	case <-timeout:
		other.worker.Stop()
	}
}

func cleanupWorkerFieldBeforeSend(owner *opaqueWorkerFields, timeout <-chan struct{}) {
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		owner.worker.Run()
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-timeout:
		owner.worker.Stop()
	}
}

func cleanupWorkerFieldBeforeReceive(owner *opaqueWorkerFields, timeout <-chan struct{}) {
	done := make(chan struct{})
	work := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		owner.worker.Run()
		<-work
		close(done)
	}()
	select {
	case <-done:
	case <-timeout:
		owner.worker.Stop()
	}
}

func cleanupWorkerFieldBypassed(owner *opaqueWorkerFields, timeout <-chan struct{}, skip bool) {
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		owner.worker.Run()
		close(done)
	}()
	if skip {
		return
	}
	select {
	case <-done:
	case <-timeout:
		owner.worker.Stop()
	}
}

type visibleWorker struct{}

func (*visibleWorker) Run()  {}
func (*visibleWorker) Stop() {}

type visibleWorkerFields struct{ worker *visibleWorker }

// Matching method names do not make a visible ignored operation opaque.
func visibleFieldCallDoesNotTransfer(owner *visibleWorkerFields, timeout <-chan struct{}) {
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		owner.worker.Run()
		close(done)
	}()
	select {
	case <-done:
	case <-timeout:
		owner.worker.Stop()
	}
}

func reassignedWorkerOwnerDoesNotTransfer(owner, other *opaqueWorkerFields, timeout <-chan struct{}) {
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		owner.worker.Run()
		close(done)
	}()
	owner = other
	select {
	case <-done:
	case <-timeout:
		owner.worker.Stop()
	}
}
