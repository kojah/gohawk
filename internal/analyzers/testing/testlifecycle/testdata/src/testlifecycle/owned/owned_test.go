package owned

import (
	"context"
	"testing"
)

func waitForCancellation(ctx context.Context) { <-ctx.Done() }

func TestExplicitlyOwnedWorker(t *testing.T) {
	go waitForCancellation(context.Background())
}
