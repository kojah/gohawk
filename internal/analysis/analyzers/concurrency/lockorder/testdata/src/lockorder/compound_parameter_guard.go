package lockorder

import "sync"

func compoundParameterGuard(mu *sync.Mutex, a, b, c, d, e *int, work bool) {
	if a != nil || b != nil || c != nil || d != nil || e != nil {
		mu.Lock()
	}
	if work && a != nil {
		*a = 1
	}
	if a != nil || b != nil || c != nil || d != nil || e != nil {
		mu.Unlock()
	}
}

func compoundGuardMissingAlternative(mu *sync.Mutex, a, b, c *int) {
	if a != nil || b != nil || c != nil {
		mu.Lock()
	}
	if a != nil || b != nil {
		mu.Unlock()
	}
	return // want "lock .* is not released on this return path"
}

func compoundGuardChangedValue(mu *sync.Mutex, a, b *int) {
	if a != nil {
		mu.Lock()
	}
	a = b
	if a != nil {
		mu.Unlock()
	}
	return // want "lock .* is not released on this return path"
}
