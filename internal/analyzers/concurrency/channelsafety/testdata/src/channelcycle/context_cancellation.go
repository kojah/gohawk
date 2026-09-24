package channelcycle

import "context"

// Context cancellation remains an alternative, never a second unconditional
// receive. Even without proving it will be chosen, it defeats this exact
// two-channel cycle proof. A cancellation request is not a worker join.
func contextCancellationAlternative() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := make(chan int), make(chan int)
	go func() {
		select {
		case b <- 1:
		case <-ctx.Done():
			return
		}
		<-a
	}()
	a <- 1
	<-b
}
