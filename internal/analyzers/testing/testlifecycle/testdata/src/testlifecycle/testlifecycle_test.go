package testlifecycle

import (
	"context"
	"testing"
)

func waitForCancellation(ctx context.Context) { <-ctx.Done() }

//gohawk:example flagged Detached test goroutine
func TestDetachedWorker(t *testing.T) {
	go waitForCancellation(context.Background()) // want "test-owned goroutine uses a never-cancelled context"
}

//gohawk:example end

//gohawk:example ok
func TestOwnedWorker(t *testing.T) {
	go waitForCancellation(t.Context())
}

//gohawk:example end

func TestDetachedClosure(t *testing.T) {
	ctx := context.WithValue(context.Background(), "fixture", true) // want "test-owned goroutine uses a never-cancelled context"
	go func() { <-ctx.Done() }()
}

func TestExplicitCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go waitForCancellation(ctx)
	cancel()
}

func helperWithoutTestingHandle() {
	go waitForCancellation(context.Background())
}

func TestJoinedFiniteWork(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = context.Background()
	}()
	<-done
}
