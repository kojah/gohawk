package cancellationownership

import (
	"context"
	"errors"
)

// Deferred literals that capture the cancel function. A literal deferred
// directly, that calls cancel on every one of its returns, releases it. One
// that calls cancel only while the named error result is non-nil is judged
// at each return by the value that return stores, as grpc-go's ALTS
// handshake cancelled only on error before its fix:
// https://github.com/grpc/grpc-go/commit/db35da8bc5e8dcfcb57b94e9be0fba306710cc77
// Any other use of a literal that captures cancel stays unknown.

func useContext(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.New("done")
	}
	return nil
}

func cancelledByDeferredLiteral(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	defer func() { cancel() }()
	return useContext(ctx)
}

func cancelledOnlyOnError(parent context.Context) (err error) {
	ctx, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	defer func() {
		if err != nil {
			cancel()
		}
	}()
	if err = useContext(ctx); err != nil {
		return err
	}
	return nil
}

// Whether useContext returns nil is not known, so neither is whether the
// literal cancels.
func cancelledOnErrorOfUnknownResult(parent context.Context) (err error) {
	ctx, cancel := context.WithCancel(parent)
	defer func() {
		if err != nil {
			cancel()
		}
	}()
	return useContext(ctx)
}

func cancelledOnSuccess(parent context.Context) (err error) {
	ctx, cancel := context.WithCancel(parent)
	defer func() {
		if err == nil {
			cancel()
		}
	}()
	_ = ctx
	return nil
}

// A guard on a local flag is data-dependent and stays unknown.
func cancelledUnlessKept(parent context.Context, keep bool) {
	ctx, cancel := context.WithCancel(parent)
	kept := false
	defer func() {
		if !kept {
			cancel()
		}
	}()
	if keep {
		kept = true
	}
	_ = ctx
}

// A literal that captures cancel and is launched may run after return.
func cancelledByLaunchedLiteral(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	go func() {
		<-ctx.Done()
		cancel()
	}()
}

// A literal that captures cancel and is handed to an unknown callee.
func cancelledByHandedLiteral(parent context.Context, run func(func())) {
	_, cancel := context.WithCancel(parent)
	run(func() { cancel() })
}

// The function also calls cancel itself through the captured variable, so
// every return is covered whatever the literal does.
func cancelledDirectlyAndCaptured(parent context.Context) (err error) {
	ctx, cancel := context.WithCancel(parent)
	defer func() {
		if err != nil {
			cancel()
		}
	}()
	if err = useContext(ctx); err != nil {
		return err
	}
	cancel()
	return nil
}
