package lockorder

import "sync"

// Every normal path retains the read lock, including an opaque error result.
// The caller owns the critical section regardless of the result's value.
func acquireReadForCaller(mu *sync.RWMutex, refresh bool, lookup func() error) error {
	mu.RLock()
	var err error
	if refresh {
		mu.RUnlock()
		mu.Lock()
		err = lookup()
		mu.Unlock()
		mu.RLock()
	}
	return err
}

func useReadHandoff(mu *sync.RWMutex, refresh bool, lookup func() error) error {
	err := acquireReadForCaller(mu, refresh, lookup)
	mu.RUnlock()
	return err
}

// One merged SSA return is reached both holding and without holding the lock.
// A retained-path witness cannot establish the contract for every path.
func mergedHeldAndReleased(mu *sync.Mutex, release bool) error {
	mu.Lock()
	if release {
		mu.Unlock()
	}
	return nil // want "not released on this return path"
}

func reacquiredReadMissingRelease(mu *sync.RWMutex, lookup func() error, release bool) error {
	mu.RLock()
	mu.RUnlock()
	err := lookup()
	mu.RLock()
	if release {
		mu.RUnlock()
	}
	return err // want "not released on this return path"
}
