package goroutineownership

import "time"

// This file covers the counted select drain: a loop that runs exactly N times
// around a blocking select whose arms receive from worker channels. A receive
// completes only against a send, so when every channel has at most one send
// and the loop runs at least once per channel, leaving the loop proves every
// worker sent. Anything that breaks the count or adds a sender is reported as
// before.

func sendOnce(out chan<- string, value string) { out <- value }

func drainedByCountedSelect() {
	first := make(chan string)
	second := make(chan string)
	third := make(chan string)
	go func() { first <- "first" }()
	go func() { second <- "second" }()
	go func() { third <- "third" }()
	for i := 0; i < 3; i++ {
		select {
		case <-first:
		case <-second:
		case <-third:
		}
	}
}

func drainedByCountedSelectWithHelper() {
	first := make(chan string)
	second := make(chan string)
	go sendOnce(first, "first")
	go sendOnce(second, "second")
	for i := 0; i < 2; i++ {
		select {
		case value := <-first:
			if value == "" {
				continue
			}
			println(value)
		case <-second:
		}
	}
}

func drainNeedsEveryIteration() {
	first := make(chan string)
	second := make(chan string)
	go func() { first <- "first" }()   // want "goroutine is not joined on every return path"
	go func() { second <- "second" }() // want "goroutine is not joined on every return path"
	for i := 0; i < 1; i++ {
		select {
		case <-first:
		case <-second:
		}
	}
}

func drainWithDefaultDoesNotWait() {
	first := make(chan string)
	second := make(chan string)
	go func() { first <- "first" }()   // want "goroutine is not joined on every return path"
	go func() { second <- "second" }() // want "goroutine is not joined on every return path"
	for i := 0; i < 2; i++ {
		select {
		case <-first:
		case <-second:
		default:
		}
	}
}

func drainWithBreakStopsEarly(stop bool) {
	first := make(chan string)
	second := make(chan string)
	go func() { first <- "first" }()   // want "goroutine is not joined on every return path"
	go func() { second <- "second" }() // want "goroutine is not joined on every return path"
	for i := 0; i < 2; i++ {
		select {
		case <-first:
		case <-second:
		}
		if stop {
			break
		}
	}
}

func drainWithTimeoutMayGiveUp() {
	first := make(chan string)
	second := make(chan string)
	go func() { first <- "first" }()   // want "goroutine is not joined on every return path"
	go func() { second <- "second" }() // want "goroutine is not joined on every return path"
	for i := 0; i < 2; i++ {
		select {
		case <-first:
		case <-second:
		case <-time.After(time.Second):
		}
	}
}

func drainWithDynamicCount(count int) {
	first := make(chan string)
	second := make(chan string)
	go func() { first <- "first" }()   // want "goroutine is not joined on every return path"
	go func() { second <- "second" }() // want "goroutine is not joined on every return path"
	for i := 0; i < count; i++ {
		select {
		case <-first:
		case <-second:
		}
	}
}

// Two sends on one channel can satisfy both iterations, leaving the second
// channel's worker blocked forever. The two senders sharing a channel stay
// unknown under the shared-signal rules that predate this proof.
func drainWithSecondSender() {
	first := make(chan string)
	second := make(chan string)
	go func() { first <- "first" }()
	go func() { first <- "again" }()
	go func() { second <- "second" }() // want "goroutine is not joined on every return path"
	for i := 0; i < 2; i++ {
		select {
		case <-first:
		case <-second:
		}
	}
}
