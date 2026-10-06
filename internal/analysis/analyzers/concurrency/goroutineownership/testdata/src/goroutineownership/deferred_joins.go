package goroutineownership

import "sync"

// This file covers exact joins owned by deferred callbacks. The callback must
// remain unambiguous and wait for the same group settled by the worker.

func deferredClosureJoinsWorker() {
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }()
	defer func() { group.Wait() }()
}

func deferredOnceCallbackJoinsWorker() {
	var group sync.WaitGroup
	group.Add(1)
	stop := sync.OnceFunc(func() { group.Wait() })
	defer stop()
	go func() { defer group.Done() }()
}

func conditionalDeferredCallbackDoesNotJoin(wait bool) {
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }() // want "goroutine is not joined on every return path"
	defer func() {
		if wait {
			group.Wait()
		}
	}()
}

func deferredCallbackWaitsForDifferentGroup() {
	var workerGroup sync.WaitGroup
	var otherGroup sync.WaitGroup
	workerGroup.Add(1)
	go func() { defer workerGroup.Done() }() // want "goroutine is not joined on every return path"
	defer func() { otherGroup.Wait() }()
}

func reassignedDeferredCallbackIsUnknown(replace bool) {
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }()
	stop := sync.OnceFunc(func() { group.Wait() })
	if replace {
		stop = func() {}
	}
	defer stop()
}

func immediateOnceCallbackJoinsWorker() {
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }()
	stop := sync.OnceFunc(func() { group.Wait() })
	stop()
}
