package callerbounds

import (
	"context"
	"sync"
)

func callerStop(stop <-chan struct{}) {
	done := make(chan bool)
	go func() { <-stop; done <- true }()
}

func callerContext(ctx context.Context) {
	done := make(chan bool)
	go func() { <-ctx.Done(); done <- true }()
}

func receiveStop(stop <-chan struct{}) { <-stop }
func receiveContext(ctx context.Context) { <-ctx.Done() }

// The receive-only channel type guard does not follow a captured pointer cell
// into the helper. The local completion send remains a diagnostic control.
func capturedStopHelper(stop <-chan struct{}) {
	done := make(chan bool)
	go func() { receiveStop(stop); done <- true }() // want "goroutine is not joined on every return path"
}

func callerStopHelper(stop <-chan struct{}) {
	done := make(chan bool)
	go func(input <-chan struct{}) { receiveStop(input); done <- true }(stop)
}

func callerContextHelper(ctx context.Context) {
	done := make(chan bool)
	go func() { receiveContext(ctx); done <- true }()
}

// Caller-owned completion handles express an independent transfer contract.
func callerCompletion(stop <-chan struct{}, done chan<- bool) {
	go func() { <-stop; done <- true }()
}

func callerGroup(ctx context.Context, group *sync.WaitGroup) {
	go func() { defer group.Done(); <-ctx.Done() }()
}

func exactJoin() {
	done := make(chan bool)
	go func() { done <- true }()
	<-done
}

func ignoredContext(ctx context.Context) {
	done := make(chan bool)
	go func() { done <- true }() // want "goroutine is not joined on every return path"
}

func localStop() {
	stop := make(chan struct{})
	done := make(chan bool)
	go func() { <-stop; done <- true }() // want "goroutine is not joined on every return path"
}
