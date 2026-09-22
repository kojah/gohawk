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

// The two phis share possible sources but select opposite channels. A
// possible-alias match is not evidence of a send on the closed channel.
func closeAndSendDifferentChannels(choose bool) {
	a, b := make(chan int, 1), make(chan int, 1)
	closed, sent := a, b
	if choose {
		closed, sent = b, a
	}
	close(closed)
	sent <- 1
}

func closeAndSendSameSelectedChannel(choose bool) {
	a, b := make(chan int, 1), make(chan int, 1)
	selected := a
	if choose {
		selected = b
	}
	close(selected)
	selected <- 1 // want "send follows close of channel"
}

func replacedChannelCell() {
	ch := make(chan int, 1)
	cell := &ch
	close(*cell)
	*cell = make(chan int, 1)
	*cell <- 1
}
