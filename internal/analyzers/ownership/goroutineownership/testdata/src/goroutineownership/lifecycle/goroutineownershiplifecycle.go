package goroutineownershiplifecycle

import "context"

func InvokeSynchronously(callback func()) { callback() }

func InvokeAsynchronously(callback func()) {
	go callback() // want "goroutine is not joined on every return path"
}

func contextBoundWorker(ctx context.Context) {
	go func() { // want "goroutine is not joined on every return path"
		<-ctx.Done()
	}()
}

type lifecycleOwner struct{}

func (*lifecycleOwner) run()  {}
func (*lifecycleOwner) Stop() {}

func lifecycleOwned() {
	owner := &lifecycleOwner{}
	go owner.run()
	defer owner.Stop()
}

func explicitlyJoined() {
	done := make(chan struct{})
	go func() { close(done) }()
	<-done
}
