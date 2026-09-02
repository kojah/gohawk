package contextpolicy

import (
	"context"
)

//gohawk:example flagged Context parameter order
func LoadUser(id string, ctx context.Context) error { // want "context.Context must be first parameter"
	return nil
}

//gohawk:example end

//gohawk:example flagged Context stored in a struct
type Request struct {
	Context context.Context // want "do not store context.Context in a struct"
}

//gohawk:example end

//gohawk:example flagged Nil context argument
func acceptContext(context.Context) {}

func loadWithoutContext() {
	acceptContext(nil) // want "do not pass nil context.Context"
}

//gohawk:example end

//gohawk:example ok
func LoadUserCorrectly(ctx context.Context, id string) error {
	return nil
}

//gohawk:example end
