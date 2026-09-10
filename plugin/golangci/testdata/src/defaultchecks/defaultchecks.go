package defaultchecks

import "sync"

var mu sync.Mutex

func missingRelease(skip bool) {
	mu.Lock()
	if skip {
		return // want "is not released on this return path"
	}
	mu.Unlock()
}
