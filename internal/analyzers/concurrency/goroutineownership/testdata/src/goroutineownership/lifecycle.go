package goroutineownership

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"

	imported "goroutineownership/lifecycle"
)

// This file covers direct lifecycle ownership, explicit background transfers,
// testing synctest scopes, and basic signal or WaitGroup joins.
// Launches with no completion promise are accepted in every mode. Missing
// ownership alone is not a defect; the broad detached audit has been retired.
// Gap: an early Done may be a mistaken join or an intentional readiness
// notification. Without a separate completion promise neither is reported.

func detached() {
	go func() {}()
}

func synchronousCallbackWrapper(callback func()) {
	callback()
}

func asynchronousCallbackWrapper(callback func()) {
	go callback()
}

func callerContextThroughSynchronousWrapper(ctx context.Context) {
	go synchronousCallbackWrapper(func() {
		<-ctx.Done()
	})
}

func callerContextThroughAsynchronousWrapper(ctx context.Context) {
	go asynchronousCallbackWrapper(func() {
		<-ctx.Done()
	})
}

func callerContextThroughImportedSynchronousWrapper(ctx context.Context) {
	go imported.InvokeSynchronously(func() {
		<-ctx.Done()
	})
}

func callerContextThroughImportedAsynchronousWrapper(ctx context.Context) {
	go imported.InvokeAsynchronously(func() {
		<-ctx.Done()
	})
}

func joinedBySynctest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		go func() {}()
	})
}

func unrelatedSynctestName(t *testing.T) {
	test := func(*testing.T, func(*testing.T)) {}
	test(t, func(t *testing.T) {
		go func() {}()
	})
}

func documentedBackgroundWorker() {
	//gohawk:ignore goroutineownership fixture intentionally transfers the worker to process lifetime
	go func() {}()
}

func undocumentedBackgroundWorker() {
	//gohawk:ignore goroutineownership
	go func() {}()
}

type lifecycleOwner struct{}

func (*lifecycleOwner) run() {}

func (*lifecycleOwner) Wait() {}

func (owner *lifecycleOwner) startMethod() {
	go owner.run()
}

func (owner *lifecycleOwner) startClosure() {
	go func() { owner.run() }()
}

func (owner *lifecycleOwner) startWithUnobservedSignal() {
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		owner.run()
		close(done)
	}()
}

func startCallerOwned(owner *lifecycleOwner) {
	go owner.run()
}

func startCallerOwnedClosure(owner *lifecycleOwner) {
	go func() { owner.run() }()
}

func startLocallyOwned() {
	owner := &lifecycleOwner{}
	go owner.run()
	defer owner.Wait()
}

func conditionallyStopLocal(stop bool) {
	owner := &lifecycleOwner{}
	go owner.run()
	if stop {
		owner.Stop()
	}
}

func stoppedBeforeSpawnDoesNotOwnLaterWorker() {
	owner := &lifecycleOwner{}
	owner.Stop()
	go owner.run()
}

func (*lifecycleOwner) Stop() {}

func returnedSignal() <-chan struct{} {
	done := make(chan struct{})
	go func() { close(done) }()
	return done
}

func joinedBySignal() {
	done := make(chan struct{}, 1)
	go func() { done <- struct{}{} }()
	<-done
}

type channelLifecycleConfig struct {
	updates <-chan int
}

func channelRangeOwnsLifecycle(config *channelLifecycleConfig) {
	go func() {
		for range config.updates {
		}
	}()
}

func nonChannelRangeDoesNotOwnLifecycle(items []int) {
	go func() {
		for range items {
		}
	}()
}

func joinedByWaitGroup() {
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
	}()
	group.Wait()
}

func joinedByDominatingDeferredWaitGroup() {
	var group sync.WaitGroup
	defer group.Wait()
	group.Add(1)
	go func() {
		defer group.Done()
	}()
}

func differentDeferredWaitGroupDoesNotJoin() {
	var group, other sync.WaitGroup
	defer other.Wait()
	group.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		defer group.Done()
	}()
}

func conditionalDeferredWaitGroupDoesNotJoin(wait bool) {
	var group sync.WaitGroup
	if wait {
		defer group.Wait()
	}
	group.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		defer group.Done()
	}()
}

func waitGroupWork() {}

func joinedByTerminalWaitGroupDone() {
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		waitGroupWork()
		group.Done()
	}()
	group.Wait()
}

func workerLocalWaitGroupDoesNotCreateJoinObligation() {
	go func() {
		var local sync.WaitGroup
		local.Add(1)
		local.Done()
		waitGroupWork()
	}()
}

func launchedWaitGroupDoneIsReadinessOnly() {
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		go group.Done()
		waitGroupWork()
	}()
	group.Wait()
}

func registeredWaitGroupWithoutWait() {
	var group sync.WaitGroup
	group.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		defer group.Done()
	}()
}

type closingOwner interface {
	Serve()
	Close() error
}

type concreteClosingOwner struct{}

func (*concreteClosingOwner) Serve() {}

func (*concreteClosingOwner) Close() error { return nil }

func deferredCloseOfAssertedOwnerBoundsWorker(owner closingOwner) {
	concrete := owner.(*concreteClosingOwner)
	defer concrete.Close()
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		owner.Serve()
	}()
}

func deferredCloseOfOtherAssertedValueDoesNotBoundWorker(owner, other closingOwner) {
	concrete := other.(*concreteClosingOwner)
	defer concrete.Close()
	var group sync.WaitGroup
	group.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		defer group.Done()
		owner.Serve()
	}()
}
