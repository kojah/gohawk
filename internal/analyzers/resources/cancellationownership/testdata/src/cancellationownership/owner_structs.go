package cancellationownership

import (
	"context"
	"errors"
)

// A cancel function stored into a field of a struct this function allocates,
// directly or through a closure that captures it, belongs to that struct.
// Returning the struct hands the cancel to the caller; a return that drops
// the struct leaves the cancel uncalled.

type ownerWorker struct {
	cancel context.CancelFunc
	stop   func()
	name   string
}

func ownerCheck(ctx context.Context) error { return ctx.Err() }

func ownerRegister(*ownerWorker) {}

var ownerGlobal *ownerWorker

// The error return drops the struct holding the cancel.
func newOwnerDirect(ctx context.Context) (*ownerWorker, error) {
	ctx, cancel := context.WithCancel(ctx) // want "cancel function from context.WithCancel is not called on every return path"
	worker := &ownerWorker{cancel: cancel, name: "direct"}
	if err := ownerCheck(ctx); err != nil {
		return nil, err
	}
	return worker, nil
}

// The constructor-wrapper shape: a closure that cancels is stored in a field
// the owner never invokes on the error return.
func newOwnerClosure(ctx context.Context) (*ownerWorker, error) {
	ctx, cancel := context.WithCancel(ctx) // want "cancel function from context.WithCancel is not called on every return path"
	worker := &ownerWorker{}
	worker.stop = func() { cancel() }
	if err := ownerCheck(ctx); err != nil {
		return nil, errors.New("setup failed")
	}
	return worker, nil
}

// Other fields of the owner are written before the dropping return.
func newOwnerOtherFields(ctx context.Context, name string) (*ownerWorker, error) {
	ctx, cancel := context.WithCancel(ctx) // want "cancel function from context.WithCancel is not called on every return path"
	worker := &ownerWorker{cancel: cancel}
	worker.name = name
	if err := ownerCheck(ctx); err != nil {
		return nil, err
	}
	return worker, nil
}

// Accepted: the owner is returned on every path.
func newOwnerAlwaysReturned(ctx context.Context) (*ownerWorker, error) {
	ctx, cancel := context.WithCancel(ctx)
	worker := &ownerWorker{cancel: cancel}
	return worker, ownerCheck(ctx)
}

// Accepted: the error path calls the cancel directly.
func newOwnerCancelOnError(ctx context.Context) (*ownerWorker, error) {
	ctx, cancel := context.WithCancel(ctx)
	worker := &ownerWorker{stop: func() { cancel() }}
	if err := ownerCheck(ctx); err != nil {
		cancel()
		return nil, err
	}
	return worker, nil
}

// Accepted: the owner is handed to code that may keep it.
func newOwnerRegistered(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	worker := &ownerWorker{cancel: cancel}
	ownerRegister(worker)
	return ownerCheck(ctx)
}

// Accepted: the owner is published to package storage.
func newOwnerPublished(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	worker := &ownerWorker{cancel: cancel}
	ownerGlobal = worker
	return ownerCheck(ctx)
}

// Accepted: the closure is handed to opaque code rather than to an owner.
func closureHandedAway(ctx context.Context, register func(func())) error {
	ctx, cancel := context.WithCancel(ctx)
	register(func() { cancel() })
	return ownerCheck(ctx)
}

// Accepted: the captured cell is written twice, so which cancel the closure
// calls is not one value.
func cellWrittenTwice(ctx context.Context, again bool) (*ownerWorker, error) {
	ctx, cancel := context.WithCancel(ctx)
	if again {
		ctx, cancel = context.WithCancel(ctx)
	}
	worker := &ownerWorker{stop: func() { cancel() }}
	if err := ownerCheck(ctx); err != nil {
		return nil, err
	}
	return worker, nil
}

// Accepted: the cell is written after the closure captures it.
func cellWrittenAfterCapture(ctx context.Context) (*ownerWorker, error) {
	var cancel context.CancelFunc
	worker := &ownerWorker{stop: func() { cancel() }}
	ctx, cancel = context.WithCancel(ctx)
	if err := ownerCheck(ctx); err != nil {
		return nil, err
	}
	return worker, nil
}

// Accepted: the closure captures a value that is not the cancel; the cancel
// itself is deferred.
func cellHoldsOtherValue(ctx context.Context, other func()) (*ownerWorker, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	worker := &ownerWorker{stop: func() { other() }}
	if err := ownerCheck(ctx); err != nil {
		return nil, err
	}
	return worker, nil
}

// Accepted: the cell is read before the cancel is stored in it.
func cellReadBeforeStore(ctx context.Context) (*ownerWorker, error) {
	var cancel context.CancelFunc
	before := cancel
	_ = before
	ctx, cancel = context.WithCancel(ctx)
	worker := &ownerWorker{stop: func() { cancel() }}
	if err := ownerCheck(ctx); err != nil {
		return nil, err
	}
	return worker, nil
}

// Accepted: the cancel is read back out of the owner and called on the error
// path. The call through the field is not an exact release, so the obligation
// stays unknown rather than lost.
func ownerCancelThroughField(ctx context.Context) (*ownerWorker, error) {
	ctx, cancel := context.WithCancel(ctx)
	worker := &ownerWorker{cancel: cancel}
	if err := ownerCheck(ctx); err != nil {
		worker.cancel()
		return nil, err
	}
	return worker, nil
}
