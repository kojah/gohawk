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

func finishIfGroupPresent(group *sync.WaitGroup) {
	if group != nil {
		defer group.Done()
	}
}

// A nil actual cannot establish a WaitGroup completion promise merely because
// the callee has a non-nil branch. OpenIM uses a nil group for fire-and-forget
// launches and a real group for its separately joined launches:
// https://github.com/openimsdk/openim-sdk-core/blob/061ac673ffa31f4d863651fdffee7882609a5f62/internal/conversation_msg/notification.go#L441-L469
func nilWaitGroupArgumentDoesNotObligate() {
	go finishIfGroupPresent(nil)
}

func realWaitGroupArgumentNeedsJoin() {
	var group sync.WaitGroup
	group.Add(1)
	go finishIfGroupPresent(&group) // want "goroutine is not joined on every return path"
}

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
