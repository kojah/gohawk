package channelownership

//gohawk:example flagged
func consume(events chan int) {
	defer close(events) // want "do not close a channel received from caller"
	for range events {
	}
}

//gohawk:example end

//gohawk:example ok
func consumeSafely(events <-chan int) {
	for range events {
	}
}

//gohawk:example end

func startOwnedSignal() {
	done := make(chan struct{})
	go finishOwnedSignal(done)
}

func finishOwnedSignal(done chan struct{}) { close(done) }

func guardedSafeClose[T any](channel chan T) {
	select {
	case <-channel:
	default:
		close(channel)
	}
}
