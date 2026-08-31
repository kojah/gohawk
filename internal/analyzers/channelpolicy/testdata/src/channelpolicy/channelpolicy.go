package channelpolicy

func wideQueue() {
	events := make(chan int, 3) // want "channel capacity 3 requires a bounded rationale"
	close(events)
	events <- 1 // want "send follows close of channel"
}

func closeBorrowed(events chan int) {
	close(events) // want "do not close a channel received from caller"
}

func closeBorrowedAlias(events chan int) {
	alias := events
	close(alias) // want "do not close a channel received from caller"
}

func startOwnedSignal() {
	done := make(chan struct{})
	go finishOwnedSignal(done)
}

func finishOwnedSignal(done chan struct{}) {
	close(done)
}

func localSignal() {
	done := make(chan struct{}, 1)
	close(done)
}

func runtimeBound(size int) {
	_ = make(chan int, size)
}

func documentedQueue() {
	// Bounded: one slot per fixed worker.
	_ = make(chan int, 3)
}

func sendAfterAliasClose() {
	events := make(chan int)
	alias := events
	close(alias)
	events <- 1 // want "send follows close of channel"
}

func sendAfterBranchedClose(closeNow bool) {
	events := make(chan int)
	if closeNow {
		close(events)
	}
	if closeNow {
		events <- 1 // want "send follows close of channel"
	}
}

func deferredCloseAfterSends(values []int) <-chan int {
	events := make(chan int)
	go func() {
		defer close(events)
		for _, value := range values {
			events <- value
		}
	}()
	return events
}

func closeEachChannel(channels []chan error, err error) {
	for _, channel := range channels {
		channel <- err
		close(channel)
	}
}
