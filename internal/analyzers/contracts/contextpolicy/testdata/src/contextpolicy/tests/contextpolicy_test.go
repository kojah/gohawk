package contextpolicy

import (
	"context"
	"testing"
)

func accept(context.Context) {}

func TestBackground(t *testing.T) {
	_ = t
	accept(context.Background())
}

func waitForCancellation(ctx context.Context) {
	<-ctx.Done()
}

func TestDetachedBackground(t *testing.T) {
	_ = t
	go waitForCancellation(context.Background())
}

func TestDetachedBackgroundClosure(t *testing.T) {
	_ = t
	ctx := context.WithValue(context.Background(), "fixture", true)
	go func() { <-ctx.Done() }()
}

func TestExplicitCancellation(t *testing.T) {
	_ = t
	ctx, cancel := context.WithCancel(context.Background())
	go waitForCancellation(ctx)
	cancel()
}

func helperWithoutTestingHandle() {
	go waitForCancellation(context.Background())
}

func finiteWorker(context.Context) {}

func TestJoinedFiniteWork(t *testing.T) {
	_ = t
	done := make(chan struct{})
	go func() {
		defer close(done)
		finiteWorker(context.Background())
	}()
	<-done
}

func TestExplicitStopAndJoin(t *testing.T) {
	_ = t
	ctx := context.Background()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
		case <-stop:
		}
	}()
	close(stop)
	<-done
}
