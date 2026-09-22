package goroutineownershipstrict

import "context"

func contextBoundWorker(ctx context.Context) {
	go func() {
		<-ctx.Done()
	}()
}

type lifecycleOwner struct{}

func (*lifecycleOwner) run()  {}
func (*lifecycleOwner) Stop() {}

func lifecycleOnly() {
	owner := &lifecycleOwner{}
	go owner.run()
	defer owner.Stop()
}

func explicitlyJoined() {
	done := make(chan struct{})
	go func() { close(done) }()
	<-done
}

func localCancellationIsNotAJoin() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		<-ctx.Done()
	}()
}
