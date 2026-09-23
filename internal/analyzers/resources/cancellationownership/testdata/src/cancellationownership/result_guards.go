package cancellationownership

// An exported helper's proven result prunes the branch it can never take,
// so an early return behind an error that is always nil is not a path that
// skips the cancel. A helper that may fail keeps the return feasible.

import (
	"context"
	"errors"

	"cancellationdep"
)

func mayFail(flag bool) error {
	if flag {
		return errors.New("failed")
	}
	return nil
}

func cancelAfterInfallibleStep(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	if err := cancellationdep.NeverFails(); err != nil {
		return
	}
	_ = ctx
	cancel()
}

func cancelAfterFallibleStep(parent context.Context, flag bool) {
	ctx, cancel := context.WithCancel(parent) // want "cancel function from context.WithCancel is not called on every return path"
	if err := mayFail(flag); err != nil {
		return
	}
	_ = ctx
	cancel()
}
