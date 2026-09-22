package goroutineownershipstrict

import "sync"

func strictRelayDoesNotAcceptQueueHandoff(queue chan<- *sync.WaitGroup, timeout <-chan struct{}) {
	var group sync.WaitGroup
	group.Add(1)
	queue <- &group
	done := make(chan struct{})
	go func() { group.Wait(); close(done) }() // want "goroutine is not joined on every return path"
	select {
	case <-done:
	case <-timeout:
	}
}
