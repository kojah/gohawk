package channelcycle

import "errors"

var errCycle = errors.New("cycle")

// Results reach the caller only after both waits. Returning a channel does not
// add a partner that could unblock the root before it returns.
func cycleWithResults(fail bool) (chan int, error) {
	a := make(chan int)
	b := make(chan int)
	go func() {
		b <- 1
		<-a
	}()
	a <- 1 // want "two goroutines wait on each other's later channel operation"
	<-b
	if fail {
		return nil, errCycle
	}
	return a, nil
}

// Matching order lets both goroutines advance, with or without a result.
func matchedWithResults() (chan int, error) {
	a := make(chan int)
	b := make(chan int)
	go func() {
		<-a
		b <- 1
	}()
	a <- 1
	<-b
	return a, nil
}
