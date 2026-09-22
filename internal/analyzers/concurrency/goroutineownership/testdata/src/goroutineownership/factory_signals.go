package goroutineownership

type iteratorOwner struct{ C chan int }

func newIteratorOwner() (*iteratorOwner, chan int) {
	ch := make(chan int)
	return &iteratorOwner{C: ch}, ch
}

func iteratorFactoryTransfersChannel() *iteratorOwner {
	owner, ch := newIteratorOwner()
	go func() { defer close(ch); ch <- 1 }()
	return owner
}

func returnedCapturedChannel() <-chan error {
	owner := struct{ result chan error }{make(chan error)}
	go func() { owner.result <- nil }()
	return owner.result
}

func unrelatedCapturedChannelDoesNotTransfer() <-chan error {
	owner := struct{ result chan error }{make(chan error)}
	other := struct{ result chan error }{make(chan error)}
	go func() { owner.result <- nil }() // want "goroutine is not joined on every return path"
	return other.result
}

type subscriptions struct{ remove chan int }

func subscriptionManager() subscriptions {
	clients := make(map[int]chan int)
	owner := subscriptions{make(chan int)}
	go func() {
		for id := range owner.remove {
			close(clients[id])
			delete(clients, id)
		}
	}()
	return owner
}

func localChannelStillNeedsOwner() {
	ch := make(chan int)
	go func() { ch <- 1 }() // want "goroutine is not joined on every return path"
}
