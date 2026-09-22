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
