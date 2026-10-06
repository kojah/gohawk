package goroutineownership

import "sync"

func updateCache() {}

//gohawk:example flagged
func refresh() {
	var group sync.WaitGroup
	group.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		defer group.Done()
		updateCache()
	}()
}

//gohawk:example end

//gohawk:example ok
func refreshSafely() {
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		updateCache()
	}()
	group.Wait()
}

//gohawk:example end
