package suppressedoptin

import (
	"context"
	"testing"
)

func TestBackground(t *testing.T) {
	_ = t
	go func(ctx context.Context) { <-ctx.Done() }(context.Background())
}
