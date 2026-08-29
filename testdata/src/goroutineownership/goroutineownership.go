package goroutineownership

import (
	"context"
	"errors"
	"sync"
)

func detached() {
	go func() {}() // want "goroutine is not joined on every return path"
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
	go owner.run() // want "goroutine is not joined on every return path"
	if stop {
		owner.Stop()
	}
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

func joinedByWaitGroup() {
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
	}()
	group.Wait()
}

func joinedByCountedReceives() {
	done := make(chan bool)
	for range 10 {
		go func() { done <- true }()
	}
	for range 10 {
		<-done
	}
}

func joinedThroughReceiveHelper() {
	done := make(chan bool)
	go func() { done <- true }()
	wait := func() { <-done }
	wait()
}

func joinedByClose() {
	done := make(chan struct{})
	go func() { close(done) }()
	<-done
}

func joinedThroughAlias() {
	done := make(chan struct{})
	alias := done
	go func() { close(alias) }()
	<-done
}

func conditionallyJoined(join bool) {
	done := make(chan struct{})
	go func() { close(done) }() // want "goroutine is not joined on every return path"
	if join {
		<-done
	}
}

type signalOwner struct {
	done <-chan struct{}
}

func transferredSignal(owner *signalOwner) {
	done := make(chan struct{})
	go func() { close(done) }()
	owner.done = done
}

type signalRegistry struct{}

func (*signalRegistry) Register(<-chan struct{}) {}

func registeredAfterSpawn(registry *signalRegistry) {
	done := make(chan struct{})
	go func() { close(done) }()
	registry.Register(done)
}

func transferredLifecycleMap(owners map[string]*lifecycleOwner) {
	owner := &lifecycleOwner{}
	go owner.run()
	owners["worker"] = owner
}

type nestedLifecycleOwner struct {
	worker *lifecycleOwner
}

func startsNestedCallerOwner(owner nestedLifecycleOwner) {
	go owner.worker.run()
}

func contextBoundWorker(ctx context.Context) {
	go func() {
		<-ctx.Done()
	}()
}

func runWithContext(ctx context.Context) {
	<-ctx.Done()
}

func contextArgumentWorker(ctx context.Context) {
	go runWithContext(ctx)
}

func runUntilStopped(stop <-chan struct{}) {
	<-stop
}

func stopChannelWorker(stop <-chan struct{}) {
	go runUntilStopped(stop)
}

func stopChannelClosure(stop <-chan struct{}) {
	go func() { <-stop }()
}

func abandonedRepeatedSend() error {
	errs := make(chan error)
	go func() {
		errs <- errors.New("first")  // want "goroutine send can block after the receiver stops waiting"
		errs <- errors.New("second") // want "goroutine send can block after the receiver stops waiting"
	}()
	return <-errs
}

func abandonedCompetingSends() error {
	errs := make(chan error)
	go func() { errs <- errors.New("first") }()  // want "goroutine send can block after the receiver stops waiting"
	go func() { errs <- errors.New("second") }() // want "goroutine send can block after the receiver stops waiting"
	return <-errs
}

func drainedSends() {
	errs := make(chan error)
	go func() {
		errs <- errors.New("first")
		errs <- errors.New("second")
	}()
	<-errs
	<-errs
}

func drainedSendsInLoop() {
	errs := make(chan error)
	go func() {
		defer close(errs)
		errs <- errors.New("first")
		errs <- errors.New("second")
	}()
	for range errs {
	}
}

func drainedSendsThroughSelects() {
	errs := make(chan error)
	go func() {
		errs <- errors.New("first")
		errs <- errors.New("second")
	}()
	select {
	case <-errs:
	}
	select {
	case <-errs:
	}
}

func oneSelectDoesNotDrainRepeatedSends() error {
	errs := make(chan error)
	go func() {
		errs <- errors.New("first")  // want "goroutine send can block after the receiver stops waiting"
		errs <- errors.New("second") // want "goroutine send can block after the receiver stops waiting"
	}()
	select {
	case err := <-errs:
		return err
	}
}

func oneNonBlockingSelectDoesNotDrainRepeatedSends() error {
	errs := make(chan error)
	go func() {
		errs <- errors.New("first")  // want "goroutine send can block after the receiver stops waiting"
		errs <- errors.New("second") // want "goroutine send can block after the receiver stops waiting"
	}()
	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

func cancellationAwareSend(ctx context.Context) error {
	errs := make(chan error)
	go func() {
		select {
		case errs <- errors.New("failed"):
		case <-ctx.Done():
		}
	}()
	return <-errs
}

func adequatelyBufferedSends() error {
	errs := make(chan error, 2)
	go func() {
		errs <- errors.New("first")
		errs <- errors.New("second")
	}()
	return <-errs
}

func sendTwice(errs chan<- error) {
	errs <- errors.New("first")  // want "goroutine send can block after the receiver stops waiting"
	errs <- errors.New("second") // want "goroutine send can block after the receiver stops waiting"
}

func abandonedNamedProducer() error {
	errs := make(chan error)
	go sendTwice(errs)
	return <-errs
}
