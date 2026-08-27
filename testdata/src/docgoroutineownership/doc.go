package docgoroutineownership

import "sync"

func updateCache() {}

//gohawk:example flagged
func refresh() {
	go updateCache() // want "goroutine is not joined on every return path"
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
