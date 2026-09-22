package goroutineownership

import (
	"context"
	"sync"
)

type relayJob struct{ group *sync.WaitGroup }

func relayQueueParticipant(queue chan<- relayJob, timeout <-chan struct{}) {
	var group sync.WaitGroup
	group.Add(1)
	queue <- relayJob{group: &group}
	done := make(chan struct{})
	go func() { group.Wait(); close(done) }()
	select {
	case <-done:
	case <-timeout:
	}
}

func relayUnrelatedQueue(queue chan<- relayJob, timeout <-chan struct{}) {
	var group, other sync.WaitGroup
	group.Add(1)
	queue <- relayJob{group: &other}
	done := make(chan struct{})
	go func() { group.Wait(); close(done) }() // want "goroutine is not joined on every return path"
	select {
	case <-done:
	case <-timeout:
	}
}

func relayCanceledParticipant(timeout <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done(); <-ctx.Done() }()
	done := make(chan struct{})
	go func() { group.Wait(); close(done) }()
	select {
	case <-done:
	case <-timeout:
	}
}

func relayUnrelatedCancellation(timeout <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var group, other sync.WaitGroup
	group.Add(1)
	other.Add(1)
	go func() { defer other.Done(); <-ctx.Done() }()
	done := make(chan struct{})
	go func() { group.Wait(); close(done) }() // want "goroutine is not joined on every return path"
	select {
	case <-done:
	case <-timeout:
	}
}

func relayIndependentWait() {
	var group sync.WaitGroup
	defer group.Wait()
	done := make(chan struct{})
	go func() { group.Wait(); close(done) }()
}

func relayExtraWorkIsNotCovered() {
	work := make(chan struct{})
	var group sync.WaitGroup
	defer group.Wait()
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		group.Wait()
		<-work
		close(done)
	}()
}

func relayPublicationIsNotCovered() {
	var group sync.WaitGroup
	defer group.Wait()
	done := make(chan struct{})
	go func() { group.Wait(); done <- struct{}{} }() // want "goroutine is not joined on every return path"
}
