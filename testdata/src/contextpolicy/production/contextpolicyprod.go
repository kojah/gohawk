package contextpolicyprod

import "context"

func misplaced(value string, ctx context.Context) {} // want "context.Context must be first parameter"

type stored struct {
	ctx context.Context // want "do not store context.Context in a struct"
}

func accept(context.Context) {}

func nilContext() {
	accept(nil) // want "do not pass nil context.Context"
}

func nilContextAlias() {
	ctx := context.Context(nil)
	accept(ctx) // want "do not pass nil context.Context"
}

func suppliedContext(ctx context.Context) {
	accept(ctx)
}
