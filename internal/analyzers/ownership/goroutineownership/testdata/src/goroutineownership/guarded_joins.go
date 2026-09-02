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

// A join guarded by a local flag assigned alongside the launch is correlated
// with the launch in a way the proof does not model, so it stays unknown.
func flagGuardedWait(items []func()) {
	var group sync.WaitGroup
	started := false
	for _, item := range items {
		started = true
		group.Add(1)
		go func() {
			defer group.Done()
			item()
		}()
	}
	if started {
		group.Wait()
	}
}

func parameterGuardedWait(item func(), wait bool) {
	var group sync.WaitGroup
	group.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		defer group.Done()
		item()
	}()
	if wait {
		group.Wait()
	}
}

// The same correlation inside a deferred closure: the flag assigned beside
// the launch decides whether the deferred Wait runs.
func deferredFlagGuardedWait(items []func(), fail func() bool) error {
	var group sync.WaitGroup
	var launched bool
	defer func() {
		if launched {
			group.Wait()
		}
	}()
	for _, item := range items {
		group.Add(1)
		go func() {
			defer group.Done()
			item()
		}()
		launched = true
		if fail() {
			return errNoWait
		}
	}
	launched = false
	group.Wait()
	return nil
}

var errNoWait = errorf("failed")

func errorf(text string) error { return &textError{text} }

type textError struct{ text string }

func (e *textError) Error() string { return e.text }
