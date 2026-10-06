package lockorder

import (
	"errors"
	"sync"
)

// Success returns the checked error SSA value, not a literal nil.
func checkedAcquireForCaller(mu *sync.Mutex, lookup func() (bool, error)) error {
	for {
		mu.Lock()
		retry, err := lookup()
		if err != nil {
			mu.Unlock()
			return err
		}
		if retry {
			mu.Unlock()
			continue
		}
		return err
	}
}

func checkedAcquireMissingRelease(mu *sync.Mutex, lookup func() error, release bool) error {
	mu.Lock()
	err := lookup()
	if err != nil {
		mu.Unlock()
		return err
	}
	if release {
		mu.Unlock()
		return err
	}
	return err // want "lock .* is not released on this return path"
}

// A success successor that is also the merge of the error branch does not
// establish nil, even though that merge dominates the later return.
func mergedErrorSuccessor(mu *sync.Mutex, err error, release bool) error {
	mu.Lock()
	if err != nil {
		_ = err.Error()
	}
	if release {
		mu.Unlock()
		return errors.New("released failure")
	}
	return err // want "lock .* is not released on this return path"
}
