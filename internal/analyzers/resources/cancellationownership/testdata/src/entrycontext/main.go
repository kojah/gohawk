// One standard context can span the process lifetime. This boundary does not
// cover repeatable constructors, helper scopes, or signal-stop registration.
package main

import (
	"context"
	"os"
	"os/signal"
	"time"
)

var controlPlane bool

func parent() context.Context
func configure(context.CancelFunc)
func run(context.Context)

func main() {
	ctx, cancel := context.WithCancel(parent())
	if controlPlane {
		configure(cancel)
	}
	run(ctx)

	causeCtx, causeCancel := context.WithCancelCause(parent())
	_ = causeCancel
	run(causeCtx)

	deadlineCtx, deadlineCancel := context.WithTimeout(parent(), time.Minute)
	defer deadlineCancel()
	run(deadlineCtx)

	for range os.Args {
		ctx, cancel := context.WithCancel(parent()) // want "cancel function from context.WithCancel is not called on every return path"
		_ = cancel
		run(ctx)
	}

	start := func() {
		ctx, cancel := context.WithCancel(parent()) // want "cancel function from context.WithCancel is not called on every return path"
		_ = cancel
		run(ctx)
	}
	start()
	helper()
	(&runner{}).main()

	ctx, stop := signal.NotifyContext(parent(), os.Interrupt) // want "cancel function from signal.NotifyContext is not called on every return path"
	_ = stop
	run(ctx)
	return
}

func helper() {
	ctx, cancel := context.WithCancel(parent()) // want "cancel function from context.WithCancel is not called on every return path"
	_ = cancel
	run(ctx)
}

type runner struct{}

func (*runner) main() {
	ctx, cancel := context.WithCancel(parent()) // want "cancel function from context.WithCancel is not called on every return path"
	_ = cancel
	run(ctx)
}
