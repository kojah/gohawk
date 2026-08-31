package blockingtest

import (
	"context"
	"testing"
)

func unsafeWait(t *testing.T, ctx context.Context, events chan int) {
	_ = t
	events <- 1 // want "channel send in context-aware test code requires cancellation-aware select"
	<-events    // want "blocking channel receive in test code requires cancellation-aware select"
}

func safeWait(t *testing.T, ctx context.Context, events chan int) {
	select {
	case events <- 1:
	case <-ctx.Done():
	}
	select {
	case <-events:
	case <-t.Context().Done():
	}
}

func safeAliasWait(t *testing.T, ctx context.Context, events chan int) {
	done := ctx.Done()
	select {
	case <-events:
	case <-done:
	}
}

func unsafeSelect(t *testing.T, ctx context.Context, events chan int) {
	select {
	case <-events: // want "blocking channel receive in test code requires cancellation-aware select"
	}
}

func safeClosedRange(t *testing.T) {
	events := make(chan int)
	close(events)
	for range events {
	}
}
