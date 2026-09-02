package ownershipproof

import (
	"context"
	"testing"
)

type completionRegistrar interface {
	Track(<-chan struct{})
}

func TestJoinedBackgroundObserver(t *testing.T) {
	done := make(chan struct{})
	go func(ctx context.Context) {
		select {
		case <-ctx.Done():
		default:
		}
		close(done)
	}(context.Background())
	<-done
}

func TestOpaqueCompletionHandoff(t *testing.T) {
	var registry completionRegistrar
	done := make(chan struct{})
	go func(ctx context.Context) {
		select {
		case <-ctx.Done():
		default:
		}
		close(done)
	}(context.Background())
	registry.Track(done)
}
