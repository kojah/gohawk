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
