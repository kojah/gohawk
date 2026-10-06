package goroutineownership

import "context"

// Repeated helpers retain distinct formal-value questions. Caller-owned
// context bounds supply unknown ownership, never a guaranteed join.

type receiveBindingOwner struct{ ctx context.Context }

func receiveSecondBinding(first, second *receiveBindingOwner) {
	<-second.ctx.Done()
}

func receiverBoundThroughSecondBinding(owner *receiveBindingOwner, join bool) {
	done := make(chan struct{})
	go func() {
		localContext, cancel := context.WithCancel(context.Background())
		cancel()
		local := &receiveBindingOwner{ctx: localContext}
		receiveSecondBinding(owner, local)
		receiveSecondBinding(local, owner)
		close(done)
	}()
	if join {
		<-done
	}
}

func localReceiverDoesNotSupplyCallerBound(owner *receiveBindingOwner, join bool) {
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		local := &receiveBindingOwner{ctx: context.Background()}
		receiveSecondBinding(owner, local)
		close(done)
	}()
	if join {
		<-done
	}
}
