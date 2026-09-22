package goroutineownership

import "context"

type contextEnvelope struct{ ctx context.Context }
type envelopeHandler interface{ Handle(*contextEnvelope) }
type envelopeProducer interface {
	Produce(*contextEnvelope, chan<- struct{})
}
type envelopeConsumer interface {
	Consume(*contextEnvelope, <-chan struct{})
}

func RetainContext(ctx context.Context) *contextEnvelope { return &contextEnvelope{ctx: ctx} }
func ignoreContext(context.Context) *contextEnvelope     { return &contextEnvelope{} }

func observedRetainedContext(ctx context.Context, handler envelopeHandler) {
	request := RetainContext(ctx)
	done := make(chan struct{})
	go func() {
		handler.Handle(request)
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func ignoredContextCannotSettle(ctx context.Context, handler envelopeHandler) {
	request := ignoreContext(ctx)
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		handler.Handle(request)
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func unrelatedContextCannotSettle(ctx, other context.Context, handler envelopeHandler) {
	request := RetainContext(other)
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		handler.Handle(request)
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func retainedContextDoesNotSettlePublication(ctx context.Context, handler envelopeHandler) {
	request := RetainContext(ctx)
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		handler.Handle(request)
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func opaqueOutputNeedsJoin(ctx context.Context, handler envelopeProducer, capacity int) {
	request := RetainContext(ctx)
	output := make(chan struct{}, capacity)
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		handler.Produce(request, output)
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func receiveOnlyOpaqueInput(ctx context.Context, handler envelopeConsumer) {
	request := RetainContext(ctx)
	input := make(chan struct{})
	done := make(chan struct{})
	go func() {
		handler.Consume(request, input)
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
