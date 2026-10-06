package asyncobservers

import "sync"

// Launched observers may own shutdown, but they do not join their caller.
// Synchronous and deferred observations retain exact completion coverage.
func launchWait(group *sync.WaitGroup) { go group.Wait() }
func wait(group *sync.WaitGroup)       { group.Wait() }
func deferWait(group *sync.WaitGroup)  { defer group.Wait() }

func directAsyncWait() {
	group := new(sync.WaitGroup)
	group.Add(1)
	go func() { defer group.Done() }()
	go group.Wait()
}

func helperAsyncWait() {
	group := new(sync.WaitGroup)
	group.Add(1)
	go func() { defer group.Done() }()
	launchWait(group)
}

func directWait() {
	group := new(sync.WaitGroup)
	group.Add(1)
	go func() { defer group.Done() }()
	group.Wait()
}

func deferredWait() {
	group := new(sync.WaitGroup)
	group.Add(1)
	go func() { defer group.Done() }()
	defer group.Wait()
}

func helperDeferredWait() {
	group := new(sync.WaitGroup)
	group.Add(1)
	go func() { defer group.Done() }()
	deferWait(group)
}

func launchedHelper() {
	group := new(sync.WaitGroup)
	group.Add(1)
	go func() { defer group.Done() }()
	go wait(group)
}

func unrelatedObserver() {
	group := new(sync.WaitGroup)
	group.Add(1)
	go func() { defer group.Done() }() // want "goroutine is not joined on every return path"
	go wait(new(sync.WaitGroup))
}
