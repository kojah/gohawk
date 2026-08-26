package contextpolicyconfig

import (
	"context"
	"testing"
)

func TestBackgroundAllowed(t *testing.T) {
	_ = t
	accept(context.Background())
}
