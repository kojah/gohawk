package contextpolicy

import (
	"context"
	"testing"
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

func waitForTestContext(ctx context.Context) { <-ctx.Done() }

//gohawk:example flagged Detached test goroutine
func TestDetachedWorker(t *testing.T) {
	go waitForTestContext(context.Background()) // want "test-owned goroutine uses a never-cancelled context"
}

//gohawk:example end

//gohawk:example ok
func LoadUserCorrectly(ctx context.Context, id string) error {
	return nil
}

func TestOwnedWorker(t *testing.T) {
	go waitForTestContext(t.Context())
}

//gohawk:example end
