package contextpolicy

import (
	"context"
	"testing"
)

func TestBackground(t *testing.T) {
	_ = t
	accept(context.Background()) // want "use t.Context"
}
