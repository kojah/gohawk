package doccancellationownership

import "context"

func doWork(context.Context) {}

//gohawk:example flagged
func work(parent context.Context) {
	ctx, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	_ = cancel
	doWork(ctx)
}

//gohawk:example end

//gohawk:example ok
func workSafely(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	doWork(ctx)
}

//gohawk:example end
