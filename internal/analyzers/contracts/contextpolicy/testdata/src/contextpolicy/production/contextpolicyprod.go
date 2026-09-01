package contextpolicyprod

import (
	"context"
	"sync"
	"testing"
	"time"
)

func misplaced(value string, ctx context.Context) {} // want "context.Context must be first parameter"

func multipleLifecycles(parent, server context.Context) {}

func testHelper(t *testing.T, ctx context.Context) {}

func benchmarkHelper(b *testing.B, ctx context.Context) {}

func misplacedAfterOrdinaryParameter(value string, parent, server context.Context) { // want "context.Context must be first parameter"
}

type stored struct {
	ctx context.Context // want "do not store context.Context in a struct"
}

type pointerContextImplementation struct {
	delegate context.Context
}

func (ctx *pointerContextImplementation) Deadline() (time.Time, bool) { return ctx.delegate.Deadline() }
func (ctx *pointerContextImplementation) Done() <-chan struct{}       { return ctx.delegate.Done() }
func (ctx *pointerContextImplementation) Err() error                  { return ctx.delegate.Err() }
func (ctx *pointerContextImplementation) Value(key any) any           { return ctx.delegate.Value(key) }

type valueContextImplementation struct {
	delegate context.Context
}

func (ctx valueContextImplementation) Deadline() (time.Time, bool) { return ctx.delegate.Deadline() }
func (ctx valueContextImplementation) Done() <-chan struct{}       { return ctx.delegate.Done() }
func (ctx valueContextImplementation) Err() error                  { return ctx.delegate.Err() }
func (ctx valueContextImplementation) Value(key any) any           { return ctx.delegate.Value(key) }

type lifecycleOwner struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type joinedLifecycleOwner struct {
	parentCtx context.Context
	wg        sync.WaitGroup
}

type RequestContext struct {
	ctx context.Context
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
