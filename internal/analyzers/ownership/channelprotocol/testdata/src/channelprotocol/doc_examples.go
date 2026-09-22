package channelprotocol

import "sync"

//gohawk:example flagged Waiting before receiving the worker's result
func waitBeforeReceive() {
	results, done := make(chan int), make(chan struct{})
	go func() {
		results <- 42
		close(done)
	}()
	<-done // want "channel wait prevents the worker's preceding send from completing"
	<-results
}
//gohawk:example end

//gohawk:example flagged Waiting for a worker blocked on its result
func waitGroupBeforeResult() {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	go func() {
		defer group.Done()
		results <- 42
	}()
	group.Wait() // want "WaitGroup wait prevents the worker's preceding send from completing"
	<-results
}
//gohawk:example end

//gohawk:example ok
func receiveBeforeWait() {
	results, done := make(chan int), make(chan struct{})
	go func() {
		results <- 42
		close(done)
	}()
	<-results
	<-done
}
//gohawk:example end
