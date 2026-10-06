package goroutineownership

import "sync"

// Gap: a trailing non-deferred Done need not promise completion of deferred
// work. Mistaken early joins are indistinguishable from readiness protocols.
func doneAfterDeferredWork(work, cleanup func()) {
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		defer cleanup()
		work()
	}()
	group.Wait()
}
