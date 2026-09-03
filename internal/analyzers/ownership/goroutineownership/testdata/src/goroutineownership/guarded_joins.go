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

// A counter stepped beside each launch guards the join the same way a flag
// does: the early return runs only when nothing was launched.
func counterGuardedWaiter(paths []string, merged chan<- string) {
	var group sync.WaitGroup
	started := 0
	for _, path := range paths {
		started++
		group.Add(1)
		go func() {
			defer group.Done()
			merged <- path
		}()
	}
	if started == 0 {
		return
	}
	go func() {
		group.Wait()
		close(merged)
	}()
}

func counterGuardedReceiveLoop(work func() int) int {
	pending := 0
	first := make(chan int)
	pending++
	go func() { first <- work() }()
	second := make(chan int)
	pending++
	go func() { second <- work() }()
	total := 0
	for i := 0; i < pending; i++ {
		select {
		case value := <-first:
			total += value
		case value := <-second:
			total += value
		}
	}
	return total
}

func unrelatedCounterDoesNotGuard(items []int) {
	done := make(chan bool)
	pending := len(items)
	go func() { done <- true }() // want "goroutine is not joined on every return path"
	if pending == 0 {
		return
	}
	<-done
}

// nilGuardedDoneSettles gives the worker a group only on the path that waits
// for it. A nil group means nothing was added and nothing waits, so the guard
// tracks whether the obligation exists rather than skipping it, and the
// deferred Done still settles every path that has a group. Vitess terminates
// queries this way, passing a group only on the shutdown path:
// https://github.com/vitessio/vitess/blob/44321d8ca0e2b2689e869bc680b6ce6402bba977/go/vt/vttablet/tabletserver/state_manager.go#L605-L631
func nilGuardedDoneSettles(group *sync.WaitGroup, work func()) {
	go func() {
		if group != nil {
			defer group.Done()
		}
		work()
	}()
}

// flagGuardedDoneDoesNotSettle guards the same Done with an unrelated flag, so
// the group can be waited on while the worker still runs.
func flagGuardedDoneDoesNotSettle(group *sync.WaitGroup, work func(), settle bool) {
	group.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		if settle {
			defer group.Done()
		}
		work()
	}()
	group.Wait()
}
