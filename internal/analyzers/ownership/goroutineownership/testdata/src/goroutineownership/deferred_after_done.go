package goroutineownership

import "sync"

// A trailing Done does not wait for deferred work. Keep this diagnostic even
// though the source looks like Done is the worker's last operation.
func doneBeforeDeferredWork(work, cleanup func()) {
	var group sync.WaitGroup
	group.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		defer cleanup()
		work()
		group.Done()
	}()
	group.Wait()
}

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
