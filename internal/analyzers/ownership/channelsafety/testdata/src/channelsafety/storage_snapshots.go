package channelsafety

func closedChannelInField() {
	ch := make(chan int, 1)
	x := struct{ ch chan int }{ch}
	close(ch)
	x.ch <- 1 // want "send follows close of channel"
}

func replacedChannelInField() {
	ch := make(chan int, 1)
	x := struct{ ch chan int }{ch}
	close(ch)
	x.ch = make(chan int, 1)
	x.ch <- 1
}

func closedChannelSnapshot() {
	ch := make(chan int, 1)
	x := [1]chan int{ch}
	saved := x[0]
	x[0] = make(chan int, 1)
	close(ch)
	saved <- 1 // want "send follows close of channel"
}
