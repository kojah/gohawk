package goroutineownership

import "sync"

// This file covers optional workers whose exact local stop channel also guards
// their exact wait. Related-looking guards, rebinding, and different groups do
// not establish the same lifecycle.

func guardedLocalStopJoin(enabled bool) {
	var stop chan struct{}
	var group sync.WaitGroup
	if enabled {
		stop = make(chan struct{})
		group.Add(1)
		go func() {
			defer group.Done()
			<-stop
		}()
	}
	if stop != nil {
		close(stop)
		group.Wait()
	}
}

func differentGuardDoesNotJoin(enabled, cleanup bool) {
	var stop chan struct{}
	var group sync.WaitGroup
	if enabled {
		stop = make(chan struct{})
		group.Add(1)
		go func() { // want "goroutine is not joined on every return path"
			defer group.Done()
			<-stop
		}()
	}
	if cleanup {
		close(stop)
		group.Wait()
	}
}

func reboundStopDoesNotJoin(enabled bool) {
	var stop chan struct{}
	var group sync.WaitGroup
	if enabled {
		stop = make(chan struct{})
		group.Add(1)
		go func() { // want "goroutine is not joined on every return path"
			defer group.Done()
			<-stop
		}()
		stop = make(chan struct{})
	}
	if stop != nil {
		close(stop)
		group.Wait()
	}
}

func differentGroupDoesNotJoin(enabled bool) {
	var stop chan struct{}
	var workerGroup sync.WaitGroup
	var otherGroup sync.WaitGroup
	if enabled {
		stop = make(chan struct{})
		workerGroup.Add(1)
		go func() { // want "goroutine is not joined on every return path"
			defer workerGroup.Done()
			<-stop
		}()
	}
	if stop != nil {
		close(stop)
		otherGroup.Wait()
	}
}

func mutateStop(*chan struct{}) {}

func opaqueStopMutationDoesNotJoin(enabled bool) {
	var stop chan struct{}
	var group sync.WaitGroup
	if enabled {
		stop = make(chan struct{})
		group.Add(1)
		go func() { // want "goroutine is not joined on every return path"
			defer group.Done()
			<-stop
		}()
		mutateStop(&stop)
	}
	if stop != nil {
		close(stop)
		group.Wait()
	}
}

func repeatedStopStoreDoesNotJoin(next func() bool) {
	var stop chan struct{}
	var group sync.WaitGroup
	for next() {
		stop = make(chan struct{})
		group.Add(1)
		go func() { // want "goroutine is not joined on every return path"
			defer group.Done()
			<-stop
		}()
	}
	if stop != nil {
		close(stop)
		group.Wait()
	}
}
