package goroutineownership

import "sync"

// The group announces startup, not shutdown; a long-lived listener has no
// completion promise merely because its readiness group is local.
func readinessListener(ready *sync.WaitGroup, failed bool) {
	ready.Done()
	if failed {
		return
	}
	for {
		waitGroupWork()
	}
}

func waitForReadiness(failed bool) {
	var ready sync.WaitGroup
	ready.Add(2)
	go readinessListener(&ready, failed)
	go readinessListener(&ready, failed)
	ready.Wait()
}

func independentCompletionStillNeedsJoin() {
	var ready, completed sync.WaitGroup
	ready.Add(1)
	completed.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		defer completed.Done()
		ready.Done()
		waitGroupWork()
	}()
	ready.Wait()
}
