package producerlifecycle

import (
	"errors"
	"time"
)

// Every use of the channel must be known. A channel handed on, kept, or used
// by another goroutine may be received from elsewhere, so the check stays
// silent; so it does for a worker launched more than once, and for a send
// on a channel the function also closes, which panics rather than blocks.

type channelHolder struct{ results chan int }

var keptChannel chan int

func forwardChannel(results chan int) { keptChannel = results }

func handedToHelper(limit time.Duration) (int, error) {
	results := make(chan int)
	go func() { results <- produceValue() }()
	forwardChannel(results)
	select {
	case value := <-results:
		return value, nil
	case <-time.After(limit):
		return 0, errors.New("timed out")
	}
}

func keptOnHolder(h *channelHolder, skip bool) int {
	results := make(chan int)
	h.results = results
	go func() { results <- produceValue() }()
	if skip {
		return 0
	}
	return <-results
}

func returnedChannel(skip bool) (chan int, int) {
	results := make(chan int)
	go func() { results <- produceValue() }()
	if skip {
		return results, 0
	}
	return results, <-results
}

func secondReceiver(skip bool) int {
	results := make(chan int)
	go func() { results <- produceValue() }()
	go func() { <-results }()
	if skip {
		return 0
	}
	return <-results
}

func launchedInLoop(count int, skip bool) int {
	results := make(chan int)
	for range count {
		go func() { results <- produceValue() }()
	}
	if skip {
		return 0
	}
	return <-results
}

func closedWhileSending(skip bool) int {
	results := make(chan int)
	go func() { results <- produceValue() }()
	if skip {
		close(results)
		return 0
	}
	return <-results
}

func channelFromParameter(results chan int, skip bool) int {
	go func() { results <- produceValue() }()
	if skip {
		return 0
	}
	return <-results
}
