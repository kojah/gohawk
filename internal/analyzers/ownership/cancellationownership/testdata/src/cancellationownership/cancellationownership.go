package cancellationownership

import (
	"cancellationdep"
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"testing"
	"time"
)

func importedHelperOwnsCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	cancellationdep.Invoke(cancel)
}

func conditionalImportedHelperIsUnknown(parent context.Context, enabled bool) {
	_, cancel := context.WithCancel(parent)
	cancellationdep.MaybeInvoke(cancel, enabled)
}

func leaked(parent context.Context) {
	ctx, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	_, _ = ctx, cancel
}

func canceled(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	defer cancel()
}

func conditional(parent context.Context, stop bool) {
	_, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	if stop {
		cancel()
	}
}

func transferred(parent context.Context) context.CancelFunc {
	_, cancel := context.WithCancel(parent)
	return cancel
}

func deferredClosure(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	defer func() { cancel() }()
}

func invokeSecond(_ context.CancelFunc, second context.CancelFunc) {
	second()
}

func exactSiblingArgumentDoesNotRelease(parent context.Context) {
	_, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	invokeSecond(cancel, func() {})
}

func invokeThroughHelper(cancel context.CancelFunc) {
	teardown(cancel, true)
}

func nestedExactHelperIsUnknown(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	invokeThroughHelper(cancel)
}

func invokeCallback(callback func()) {
	callback()
}

func capturedCallbackArgumentIsUnknown(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	invokeCallback(func() { cancel() })
}

func deferredReassignedCaptureIsUnknown(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	callback := cancel
	defer func() { callback() }()
	callback = func() {}
}

func deferredMixedCallbackIsUnknown(parent context.Context, release bool) {
	_, cancel := context.WithCancel(parent)
	callback := context.CancelFunc(func() {})
	if release {
		callback = cancel
	}
	defer callback()
}

func cancelCause(parent context.Context) {
	_, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
}

func leakedTimeout(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, time.Second) // want "cancel function from context.WithTimeout is not called on every return path"
	_, _ = ctx, cancel
}

func valueContextHasNoCancel(parent context.Context) {
	_ = context.WithValue(parent, "key", "value")
}

func goroutineOwnsCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	go func() {
		defer cancel()
	}()
}

type owner struct {
	cancel   context.CancelFunc
	callback func()
}

type cleanupRegistrar interface {
	DeferCleanup(func())
}

func registeredCleanup(registrar cleanupRegistrar, parent context.Context) {
	_, cancel := context.WithCancel(parent)
	registrar.DeferCleanup(func() { cancel() })
}

func registeredNestedCleanup(registrar cleanupRegistrar, parent context.Context) {
	_, cancel := context.WithCancel(parent)
	registrar.DeferCleanup(func() {
		func(callback func()) { callback() }(func() { cancel() })
	})
}

type cancelRegistry struct{ cancel context.CancelFunc }

func (registry *cancelRegistry) AddWorker(cancel context.CancelFunc) { registry.cancel = cancel }

func registeredCancel(registry *cancelRegistry, parent context.Context) {
	_, cancel := context.WithCancel(parent)
	registry.AddWorker(cancel)
}

func wrapCleanup(cancel context.CancelFunc) (int, func()) {
	return 0, cancel
}

func transferredToDeferredResult(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	_, cleanup := wrapCleanup(cancel)
	defer cleanup()
}

var globalCancel context.CancelFunc

func transferredToGlobal(parent context.Context) {
	_, globalCancel = context.WithCancel(parent)
}

func storeGlobal(cancel context.CancelFunc) {
	globalCancel = cancel
}

func helperGlobalStorageIsUnknown(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	storeGlobal(cancel)
}

func transferredToField(target *owner, parent context.Context) {
	_, cancel := context.WithCancel(parent)
	target.cancel = cancel
}

func transferredThroughCallback(target *owner, parent context.Context) {
	_, cancel := context.WithCancel(parent)
	target.callback = func() { cancel() }
}

func reassignedAndTransferredThroughCallback(target *owner, parent context.Context) {
	_, cancel := context.WithCancel(parent)
	cancel()
	_, cancel = context.WithCancel(parent)
	target.callback = func() { cancel() }
}

func leakedSignalContext(parent context.Context) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt) // want "cancel function from signal.NotifyContext is not called on every return path"
	_, _ = ctx, stop
}

func stoppedSignalContext(parent context.Context) {
	_, stop := signal.NotifyContext(parent, os.Interrupt)
	defer stop()
}

func afterFuncMayRun(parent context.Context) {
	_ = context.AfterFunc(parent, func() {})
}

func afterFuncOwnsCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	time.AfterFunc(time.Millisecond, cancel)
}

func testCleanupOwnsCancel(t *testing.T, parent context.Context) {
	_, cancel := context.WithCancel(parent)
	t.Cleanup(cancel)
}

func mapOwnsCancel(cancels map[string]context.CancelFunc, parent context.Context) {
	_, cancel := context.WithCancel(parent)
	cancels["worker"] = cancel
}

func settle(cancel context.CancelFunc) {
	cancel()
}

func helperOwnsCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	settle(cancel)
}

func conditionallySettle(cancel context.CancelFunc, run bool) {
	if run {
		cancel()
	}
}

func conditionalHelperDoesNotOwnCancel(parent context.Context, run bool) {
	_, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	conditionallySettle(cancel, run)
}

func startCancelWorker(cancel context.CancelFunc) {
	go func() {
		cancel()
	}()
}

func helperGoroutineOwnsCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	startCancelWorker(cancel)
}

func startCancelDirectly(cancel context.CancelFunc) {
	go cancel()
}

func directHelperGoroutineOwnsCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	startCancelDirectly(cancel)
}

func StartExportedCancelWorker(cancel context.CancelFunc) {
	go func() { cancel() }()
}

func exportedGoroutineTransferIsUnknown(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	StartExportedCancelWorker(cancel)
}

func settleInWorker(cancel context.CancelFunc) {
	cancel()
}

func startNestedCancelWorker(cancel context.CancelFunc) {
	go func() {
		settleInWorker(cancel)
	}()
}

func nestedHelperGoroutineOwnsCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	startNestedCancelWorker(cancel)
}

func conditionallyStartCancelWorker(cancel context.CancelFunc, run bool) {
	if run {
		go func() { cancel() }()
	}
}

func conditionalGoroutineTransferIsUnknown(parent context.Context, run bool) {
	_, cancel := context.WithCancel(parent)
	conditionallyStartCancelWorker(cancel, run)
}

func startConditionalCallbackWorker(cancel context.CancelFunc, run bool) {
	go func() {
		if run {
			cancel()
		}
	}()
}

func conditionalWorkerCallbackIsUnknown(parent context.Context, run bool) {
	_, cancel := context.WithCancel(parent)
	startConditionalCallbackWorker(cancel, run)
}

func callCancelWithoutWorker(cancel context.CancelFunc) {
	cancel()
}

func nonGoroutineHelperStillOwnsCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	callCancelWithoutWorker(cancel)
}

func observeWithoutWorker(cancel context.CancelFunc) {
	_ = cancel
}

func passToOpaqueHelper(cancel context.CancelFunc, enabled bool) {
	cancellationdep.MaybeInvoke(cancel, enabled)
}

func opaqueNestedHelperIsUnknown(parent context.Context, enabled bool) {
	_, cancel := context.WithCancel(parent)
	passToOpaqueHelper(cancel, enabled)
}

func cleanupByNameOnly(cancel context.CancelFunc) {
	_ = cancel
}

func namedCleanupDoesNotOwnCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	cleanupByNameOnly(cancel)
}

func nonGoroutineObserverDoesNotOwnCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	observeWithoutWorker(cancel)
}

func startUnrelatedCallbackWorker(cancel, other context.CancelFunc) {
	go func() { other() }()
}

func unrelatedWorkerCallbackDoesNotOwnCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	startUnrelatedCallbackWorker(cancel, func() {})
}

func dynamicGoroutineStarterIsUnknown(parent context.Context, start func(context.CancelFunc)) {
	_, cancel := context.WithCancel(parent)
	start(cancel)
}

func teardown(cancel context.CancelFunc, fast bool) {
	if fast {
		if cancel != nil {
			cancel()
		}
		return
	}
	done := make(chan struct{}, 1)
	select {
	case <-done:
		if cancel != nil {
			cancel()
		}
	case <-time.After(time.Millisecond):
		if cancel != nil {
			cancel()
		}
	}
}

func teardownHelperOwnsCancel(parent context.Context, fast bool) {
	_, cancel := context.WithCancel(parent)
	teardown(cancel, fast)
}

func deferredTeardownHelperOwnsCancel(parent context.Context, fast bool) {
	_, cancel := context.WithCancel(parent)
	defer func() { teardown(cancel, fast) }()
}

func observeCallback(context.CancelFunc) {}

func deferredObserverIsUnknown(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	defer func() { observeCallback(cancel) }()
}

type pointerCancelOwner struct {
	cancel *context.CancelFunc
}

func returnedPointerCancelOwner() (func(context.Context), pointerCancelOwner) {
	owner := pointerCancelOwner{cancel: new(context.CancelFunc)}
	run := func(parent context.Context) {
		_, cancel := context.WithCancel(parent)
		*owner.cancel = cancel
	}
	return run, owner
}

func localPointerDoesNotOwnCancel(parent context.Context) {
	slot := new(context.CancelFunc)
	_, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	*slot = cancel
}

func transferredToCapturedOuter(parent context.Context) func() {
	var current context.CancelFunc
	start := func() {
		_, cancel := context.WithCancel(parent)
		current = cancel
	}
	start()
	return func() {
		if current != nil {
			current()
		}
	}
}

func localAssignmentDoesNotTransfer(parent context.Context) {
	var current context.CancelFunc
	_, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	current = cancel
	_ = current
}

func optionalCancelIsCalled(parent context.Context, enabled bool) {
	var cancel context.CancelFunc
	if enabled {
		_, cancel = context.WithTimeout(parent, time.Second)
	}
	if cancel != nil {
		cancel()
	}
}

type atomicCancelOwner struct {
	cancel atomic.Pointer[context.CancelFunc]
}

func (owner *atomicCancelOwner) start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	owner.cancel.Store(&cancel)
	go owner.run(ctx)
}

func (*atomicCancelOwner) run(context.Context) {}

func atomicStorageIsUnknown(parent context.Context, owner *atomicCancelOwner) {
	_, cancel := context.WithCancel(parent)
	owner.cancel.Store(&cancel)
}
