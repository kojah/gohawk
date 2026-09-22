package waitgroupsafety

import "sync"

//gohawk:example flagged Extra completion
func extraCompletion() {
	var group sync.WaitGroup
	group.Add(1)
	group.Done()
	group.Done() // want "WaitGroup Done makes the counter negative"
}

//gohawk:example end

//gohawk:example ok
func oneCompletion() {
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }()
	group.Wait()
}

//gohawk:example end
