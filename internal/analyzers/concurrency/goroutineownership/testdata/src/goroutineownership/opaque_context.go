package goroutineownership

import "context"

// The exact canceled context crosses an opaque helper boundary. It may bound
// the worker, so the analyzer declines a lifecycle diagnostic without proving
// a join. A helper that ignores cancellation is an intentional coverage gap.
func canceledContextPassedToOpaqueWorker(run func(context.Context)) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		run(ctx)
		close(done)
	}()
	cancel()
}

func opaqueWorkerGetsDifferentContext(run func(context.Context)) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		_ = ctx.Err()
		run(context.Background())
		close(done)
	}()
}

func opaqueWorkerWithConditionalCancellation(run func(context.Context), cancelNow bool) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		run(ctx)
		close(done)
	}()
	if cancelNow {
		cancel()
	}
}
