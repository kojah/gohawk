package goroutineownership

import "sync"

// Optional completion registration does not promise an unconditional join.
// Missing waits for conditional-only Done remain outside this bounded proof.
func optionalDeferredCompletion(wait bool, work func()) {
	var group sync.WaitGroup
	if wait {
		group.Add(1)
	}
	go func() {
		if wait {
			defer group.Done()
		}
		work()
	}()
	if wait {
		group.Wait()
	}
}

func finishOptionalWork(group *sync.WaitGroup) { group.Done() }

func optionalDeferredHelperCompletion(wait bool, work func()) {
	var group sync.WaitGroup
	if wait {
		group.Add(1)
	}
	go func() {
		if wait {
			defer finishOptionalWork(&group)
		}
		work()
	}()
	if wait {
		group.Wait()
	}
}

func unconditionalDeferredHelperNeedsJoin(work func()) {
	var group sync.WaitGroup
	group.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		defer finishOptionalWork(&group)
		work()
	}()
}

func branchesBothRegisterCompletion(flag bool, work func()) {
	var group sync.WaitGroup
	group.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		if flag {
			defer finishOptionalWork(&group)
		} else {
			defer group.Done()
		}
		work()
	}()
}

func optionalGroupDoesNotHideOtherSignal(wait bool, work func()) {
	var group sync.WaitGroup
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		if wait {
			defer group.Done()
		}
		work()
		close(done)
	}()
}
