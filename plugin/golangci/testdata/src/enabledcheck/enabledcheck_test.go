package enabledcheck

import (
	"context"
	"testing"
)

func TestBackground(t *testing.T) {
	_ = t
	_ = context.Background() // want "use t.Context"
}
