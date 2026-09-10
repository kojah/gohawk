package disabledcheck

import "sync"

var mu sync.Mutex

func missingRelease(skip bool) {
	mu.Lock()
	if skip {
		return
	}
	mu.Unlock()
}
