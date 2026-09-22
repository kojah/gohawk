package producerlifecycle

import "errors"

//gohawk:example flagged
func firstResultOnly() error {
	results := make(chan error)
	go func() {
		results <- errors.New("first")
		results <- errors.New("second") // want "goroutine send can block after the receiver stops waiting"
	}()
	return <-results
}

//gohawk:example end

//gohawk:example ok
func drainResults() {
	results := make(chan error)
	go func() {
		results <- errors.New("first")
		results <- errors.New("second")
	}()
	<-results
	<-results
}

//gohawk:example end

func mutuallyExclusiveProducerSend(fail bool) error {
	results := make(chan error)
	go func() {
		if fail {
			results <- errors.New("failed")
			return
		}
		results <- nil
	}()
	return <-results
}

func competingSends() error {
	results := make(chan error)
	go func() { results <- errors.New("first") }()  // want "goroutine send can block after the receiver stops waiting"
	go func() { results <- errors.New("second") }() // want "goroutine send can block after the receiver stops waiting"
	return <-results
}

func firstTwoResultsOnly() {
	results := make(chan int)
	go func() {
		results <- 1
		results <- 2
		results <- 3 // want "goroutine send can block after the receiver stops waiting"
	}()
	<-results
	<-results
}

func branchingSecondResult(fail bool) {
	results := make(chan int)
	go func() {
		results <- 1
		if fail {
			results <- 2 // want "goroutine send can block after the receiver stops waiting"
			return
		}
		results <- 3 // want "goroutine send can block after the receiver stops waiting"
	}()
	<-results
}
