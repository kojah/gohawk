package lockorder

// An exported helper's proven result prunes the branch it can never take,
// so an early return behind an error that is always nil does not leave the
// lock held. A helper that may fail keeps the return feasible.

import (
	"errors"
	"sync"

	"orderedhelpers"
)

var resultGuardMutex sync.Mutex

func resultMayFail(flag bool) error {
	if flag {
		return errors.New("failed")
	}
	return nil
}

func unlockAfterInfallibleStep() {
	resultGuardMutex.Lock()
	if err := orderedhelpers.NeverFails(); err != nil {
		return
	}
	resultGuardMutex.Unlock()
}

func unlockAfterFallibleStep(flag bool) {
	resultGuardMutex.Lock()
	if err := resultMayFail(flag); err != nil {
		return // want "lock .* is not released on this return path"
	}
	resultGuardMutex.Unlock()
}
