package contextpolicyprod

import "context"

func misplaced(value string, ctx context.Context) {} // want "context.Context must be first parameter"

func multipleContexts(first, second context.Context) {}

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

type extendedContext interface {
	context.Context
}

func nilContextThroughInterface() {
	var ctx extendedContext
	accept(ctx) // want "do not pass nil context.Context"
}

func nilContextThroughBranches(useAlias bool) {
	var ctx context.Context
	if useAlias {
		ctx = context.Context(nil)
	} else {
		ctx = nil
	}
	accept(ctx) // want "do not pass nil context.Context"
}

func maybeNilContext(ctx context.Context, clear bool) {
	if clear {
		ctx = nil
	}
	accept(ctx)
}

func suppliedContext(ctx context.Context) {
	accept(ctx)
}
