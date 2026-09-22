package goroutineownership

import "sync"

// Alternative completion handles can follow a terminal Done only when the
// remaining deferred work is itself nonblocking completion bookkeeping.
func joinedBeforeDeferredCompletion() {
	var joined, completed sync.WaitGroup
	joined.Add(1)
	completed.Add(1)
	go func() {
		defer completed.Done()
		waitGroupWork()
		joined.Done()
	}()
	joined.Wait()
}

func joinedBeforeDeferredClose() {
	var joined sync.WaitGroup
	joined.Add(1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		waitGroupWork()
		joined.Done()
	}()
	joined.Wait()
}

func deferredWorkStillNeedsCompletion() {
	var ready, completed sync.WaitGroup
	ready.Add(1)
	completed.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		defer completed.Done()
		defer waitGroupWork()
		ready.Done()
	}()
	ready.Wait()
}

func alternativeCompletionStillNeedsAWait() {
	var joined, completed sync.WaitGroup
	joined.Add(1)
	completed.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		defer completed.Done()
		waitGroupWork()
		joined.Done()
	}()
}

// A buffered result offered before the deferred completion close must not
// become another join handle: a nonblocking poll need not observe completion.
func bufferedResultDoesNotReplaceDeferredCompletion(finish bool) {
	done := make(chan struct{})
	result := make(chan int, 1)
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		for {
			if finish {
				result <- 1
				return
			}
		}
	}()
	select {
	case <-result:
	default:
	}
}
