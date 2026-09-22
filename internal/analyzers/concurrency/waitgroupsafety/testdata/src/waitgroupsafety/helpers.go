package waitgroupsafety

import (
	"grouphelper"
	"sync"
)

func finish(group *sync.WaitGroup) { group.Done() }

func localHelper() {
	var group sync.WaitGroup
	group.Add(1)
	finish(&group)
	finish(&group) // want "WaitGroup Done makes the counter negative"
}

func importedHelper() {
	var group sync.WaitGroup
	group.Add(1)
	grouphelper.Finish(&group)
	grouphelper.Finish(&group) // want "WaitGroup Done makes the counter negative"
}

func deferredTwice() {
	var group sync.WaitGroup
	group.Add(1)
	defer group.Done() // want "WaitGroup Done makes the counter negative"
	group.Done()
}

func workerTwice() {
	var group sync.WaitGroup
	group.Add(1)
	go func() { // want "WaitGroup Done makes the counter negative"
		defer group.Done()
		group.Done()
	}()
	group.Wait()
}

func zeroCount() {
	var group sync.WaitGroup
	group.Done() // want "WaitGroup Done makes the counter negative"
}
