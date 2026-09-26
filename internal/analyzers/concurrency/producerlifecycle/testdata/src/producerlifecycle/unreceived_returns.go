package producerlifecycle

import (
	"os"
	"context"
	"errors"
	"time"
)

// Unreceived returns. A goroutine launched once that sends once, on every
// path, on an unbuffered channel only the function can receive from blocks
// forever when the function returns without receiving: the timeout select,
// the early error return. The count proof of abandoned-send cannot see this,
// because the receive exists; it is just not on every path. The buffered
// fix, a receive on every path, a drain in the timeout arm, a send that can
// give up, a send that may not happen, and a function that never returns
// are accepted.

func produceValue() int { return 42 }

func timeoutLeaksWorker(limit time.Duration) (int, error) {
	results := make(chan int)
	go func() { results <- produceValue() }() // want "goroutine blocks forever sending on results: the function can return without receiving"
	select {
	case value := <-results:
		return value, nil
	case <-time.After(limit):
		return 0, errors.New("timed out")
	}
}

func contextLeaksWorker(ctx context.Context) (int, error) {
	results := make(chan int)
	go func() { results <- produceValue() }() // want "goroutine blocks forever sending on results: the function can return without receiving"
	select {
	case value := <-results:
		return value, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func earlyReturnLeaksWorker(skip bool) int {
	results := make(chan int)
	go func() { results <- produceValue() }() // want "goroutine blocks forever sending on results: the function can return without receiving"
	if skip {
		return 0
	}
	return <-results
}

// A named worker function receives the channel as a parameter.
func sendProduced(results chan<- int) { results <- produceValue() }

func namedWorkerLeaks(skip bool) int {
	results := make(chan int)
	go sendProduced(results) // want "goroutine blocks forever sending on results: the function can return without receiving"
	if skip {
		return 0
	}
	return <-results
}

// The buffered channel is the usual fix: the send completes with nobody
// receiving.
func bufferedTimeout(limit time.Duration) (int, error) {
	results := make(chan int, 1)
	go func() { results <- produceValue() }()
	select {
	case value := <-results:
		return value, nil
	case <-time.After(limit):
		return 0, errors.New("timed out")
	}
}

func receivedOnEveryPath() int {
	results := make(chan int)
	go func() { results <- produceValue() }()
	return <-results
}

// The timeout arm drains the worker before returning.
func drainedOnTimeout(limit time.Duration) (int, error) {
	results := make(chan int)
	go func() { results <- produceValue() }()
	select {
	case value := <-results:
		return value, nil
	case <-time.After(limit):
		<-results
		return 0, errors.New("timed out")
	}
}

// The worker gives up its send when the context ends.
func workerCanGiveUp(ctx context.Context) (int, error) {
	results := make(chan int)
	go func() {
		select {
		case results <- produceValue():
		case <-ctx.Done():
		}
	}()
	select {
	case value := <-results:
		return value, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// The worker sends only sometimes.
func workerMaySkipSend(flag bool, skip bool) int {
	results := make(chan int)
	go func() {
		if flag {
			results <- produceValue()
		}
	}()
	if skip {
		return 0
	}
	return <-results
}

// A function that never returns cannot leave its worker behind.
func serveForever() {
	results := make(chan int)
	go func() { results <- produceValue() }()
	for {
		time.Sleep(time.Second)
	}
}

// Exiting the process on the other path leaves nothing blocked.
func exitsInsteadOfReturning(skip bool) int {
	results := make(chan int)
	go func() { results <- produceValue() }()
	if skip {
		os.Exit(1)
	}
	return <-results
}
