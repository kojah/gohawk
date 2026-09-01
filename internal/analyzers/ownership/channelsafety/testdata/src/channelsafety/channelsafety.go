package channelsafety

//gohawk:example flagged
func sendAfterClose() {
	events := make(chan int)
	close(events)
	events <- 1 // want "send follows close of channel"
}

//gohawk:example end

//gohawk:example ok
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

//gohawk:example end

func branchedClose(closeNow bool) {
	events := make(chan int)
	if closeNow {
		close(events)
	}
	if closeNow {
		events <- 1 // want "send follows close of channel"
	}
}
