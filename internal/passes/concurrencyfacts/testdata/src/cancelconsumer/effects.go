package cancelconsumer

import (
	"canceldependency"
	"cancelforwarder"
	"context"
)

func bound() {
	ctx, cancel := context.WithCancel(context.Background())
	cancelforwarder.Launch(ctx)
	cancelforwarder.Stop(cancel)
}

func unbound(ctx context.Context, cancel context.CancelFunc) {
	cancelforwarder.Launch(ctx)
	cancelforwarder.Stop(cancel)
}

func unusedDone(ctx context.Context) { canceldependency.Observe(ctx) }

func arbitraryCallback() {
	cancelforwarder.Stop(context.CancelFunc(func() { select {} }))
}
