package goroutineownership

import "context"

// This file covers workers bounded by caller-owned contexts or stop channels.
// Helper proofs require the worker to receive from the actual boundary signal.

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

func receiveStopThroughHelper(stop <-chan struct{}) {
	runUntilStopped(stop)
}

func receiveStopThroughNestedHelper(stop <-chan struct{}) {
	receiveStopThroughHelper(stop)
}

func selectStopThroughHelper(stop <-chan struct{}) {
	select {
	case <-stop:
	}
}

func helperStopChannelWorker(stop <-chan struct{}) {
	go receiveStopThroughHelper(stop)
}

func nestedHelperStopChannelWorker(stop <-chan struct{}) {
	go receiveStopThroughNestedHelper(stop)
}

func helperSelectStopChannelWorker(stop <-chan struct{}) {
	go selectStopThroughHelper(stop)
}

func helperWithoutStop(<-chan struct{}) {}

func helperWithoutStopWorker(stop <-chan struct{}) {
	go helperWithoutStop(stop) // want "goroutine is not joined on every return path"
}

func drainStateUpdate(updates chan int) {
	select {
	case <-updates:
	default:
	}
}

func drainStateUpdateThroughHelper(updates chan int) {
	drainStateUpdate(updates)
}

func bidirectionalStateHelperWorker(updates chan int) {
	go drainStateUpdateThroughHelper(updates) // want "goroutine is not joined on every return path"
}

var unrelatedStopChannel <-chan struct{}

func helperReceivesUnrelatedStop(<-chan struct{}) {
	<-unrelatedStopChannel
}

func unrelatedHelperStopChannelWorker(stop <-chan struct{}) {
	go helperReceivesUnrelatedStop(stop) // want "goroutine is not joined on every return path"
}

func dynamicHelperStopChannelWorker(run func(<-chan struct{}), stop <-chan struct{}) {
	go run(stop) // want "goroutine is not joined on every return path"
}
