package completionbindings

import "sync"

// Mixed and mutable completion bindings can hide genuine omissions. They do
// not establish an unconditional promise about the first possible caller value.

func worker(a, b chan bool, flag bool) {
	c := a
	if flag {
		c = b
	}
	c <- true
}
func mixedSignal(flag bool) {
	a, b := make(chan bool), make(chan bool)
	go worker(a, b, flag)
	if flag {
		<-b
	} else {
		<-a
	}
}
func groupWorker(a, b *sync.WaitGroup, flag bool) {
	g := a
	if flag {
		g = b
	}
	g.Done()
}
func mixedGroup(flag bool) {
	a, b := new(sync.WaitGroup), new(sync.WaitGroup)
	if flag {
		b.Add(1)
	} else {
		a.Add(1)
	}
	go groupWorker(a, b, flag)
	if flag {
		b.Wait()
	} else {
		a.Wait()
	}
}
func replacedCapture() {
	a, b := make(chan bool), make(chan bool)
	go func() { a = b; a <- true }()
	<-b
}

func exactSignalMissing()        { done := make(chan bool); go exactWorker(done) } // want "goroutine is not joined on every return path"
func exactSignalJoined()         { done := make(chan bool); go exactWorker(done); <-done }
func exactWorker(done chan bool) { done <- true }
func exactCaptureMissing()       { done := make(chan bool); go func() { done <- true }() }                    // want "goroutine is not joined on every return path"
func nestedCaptureMissing()      { done := make(chan bool); go func() { defer func() { done <- true }() }() } // want "goroutine is not joined on every return path"
func captureSnapshot() {
	a, b := make(chan bool), make(chan bool)
	done := a
	done = b
	go func() { done <- true }()
	<-b
}
func exactGroupMissing() { g := new(sync.WaitGroup); g.Add(1); go func() { defer g.Done() }() } // want "goroutine is not joined on every return path"
