package goroutineownership

// An exported helper's proven result prunes the branch it can never take,
// so an early return behind an error that is always nil is not a path that
// skips the join. A helper that may fail keeps the return feasible.

import (
	"errors"
	"sync"

	"resultguards"
)

func joinMayFail(flag bool) error {
	if flag {
		return errors.New("failed")
	}
	return nil
}

func joinAfterInfallibleStep() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done() }()
	if err := resultguards.NeverFails(); err != nil {
		return
	}
	wg.Wait()
}

func joinAfterFallibleStep(flag bool) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done() }() // want "goroutine is not joined on every return path"
	if err := joinMayFail(flag); err != nil {
		return
	}
	wg.Wait()
}
