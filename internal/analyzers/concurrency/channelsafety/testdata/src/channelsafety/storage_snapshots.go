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

type channelHolder struct {
	ch    chan int
	count int
}

func inspectHolder(owner *channelHolder) int { return owner.count }
func replaceHolder(owner *channelHolder)     { owner.ch = make(chan int, 1) }

var savedHolder *channelHolder

func retainHolder(owner *channelHolder) { savedHolder = owner }

func closedChannelAfterInspection() {
	ch := make(chan int, 1)
	x := channelHolder{ch: ch}
	close(ch)
	inspectHolder(&x)
	x.ch <- 1 // want "send follows close of channel"
}

func replacedChannelByHelper() {
	ch := make(chan int, 1)
	x := channelHolder{ch: ch}
	close(ch)
	replaceHolder(&x)
	x.ch <- 1
}

func retainedChannelOwner() {
	ch := make(chan int, 1)
	x := channelHolder{ch: ch}
	close(ch)
	retainHolder(&x)
	x.ch <- 1 // Escaped storage no longer proves which channel the field contains.
}
