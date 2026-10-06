package summaryjoins

import (
	"sync"
	"synchelpers"
)

func returnedWaiter() {
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }()
	defer synchelpers.ForwardWaiter(&group)()
}

func otherReturnedWaiter() {
	var group, other sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }() // want "goroutine is not joined"
	defer synchelpers.ForwardWaiter(&other)()
}

func uninvokedWaiter() {
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }()
	_ = synchelpers.ForwardWaiter(&group)
}

func asynchronousWaiter() {
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }()
	wait := synchelpers.ForwardWaiter(&group)
	go wait()
}
