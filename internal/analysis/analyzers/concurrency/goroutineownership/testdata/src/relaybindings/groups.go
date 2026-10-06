package relaybindings

import "sync"

func reassignedGroup() {
	a, b := new(sync.WaitGroup), new(sync.WaitGroup)
	group := a
	group = b
	done := make(chan struct{})
	go func() { group.Wait(); close(done) }()
	b.Wait()
}
func exactRelay() {
	group := new(sync.WaitGroup)
	done := make(chan struct{})
	go func() { group.Wait(); close(done) }()
	group.Wait()
}
func unrelatedWait(observe bool) {
	group, other := new(sync.WaitGroup), new(sync.WaitGroup)
	done := make(chan struct{})
	go func() { group.Wait(); close(done) }() // want "goroutine is not joined on every return path"
	other.Wait()
	if observe {
		<-done
	}
}
func relayWithWork(observe bool) {
	work := make(chan struct{})
	group := new(sync.WaitGroup)
	done := make(chan struct{})
	go func() { group.Wait(); <-work; close(done) }() // want "goroutine is not joined on every return path"
	group.Wait()
	if observe {
		<-done
	}
}

// A caller-owned input can bound the worker even when its extra receive
// disqualifies the independent Wait as exact relay completion.
func relayWithCallerWork(observe bool, work <-chan struct{}) {
	group := new(sync.WaitGroup)
	done := make(chan struct{})
	go func() { group.Wait(); <-work; close(done) }()
	group.Wait()
	if observe {
		<-done
	}
}
