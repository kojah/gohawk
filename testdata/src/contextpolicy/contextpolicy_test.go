package contextpolicy

import (
	"context"
	"testing"
)

func accept(context.Context) {}

func TestBackground(t *testing.T) {
	_ = t
	accept(context.Background()) // want "use t.Context"
}
