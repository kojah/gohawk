package contextpolicy

import (
	"context"
)

// Delisted from the catalog; kept so the implementation stays exercised.
func LoadUser(id string, ctx context.Context) error { // want "context.Context must be first parameter"
	return nil
}

type Request struct {
	Context context.Context // want "do not store context.Context in a struct"
}

func acceptContext(context.Context) {}

func loadWithoutContext() {
	acceptContext(nil) // want "do not pass nil context.Context"
}

func LoadUserCorrectly(ctx context.Context, id string) error {
	return nil
}
