// Package example contains illustrative inputs for the lock-analysis blog draft.
// Update deliberately leaks a lock on its early return; do not copy it as safe code.
package example

import (
	"os"
	"sync"
)

// Update demonstrates an early return that skips the unlock.
func Update(mu *sync.Mutex, ready bool) {
	mu.Lock()
	if !ready {
		return
	}
	mu.Unlock()
}

// CloseFile closes file and returns the close error to its caller.
func CloseFile(file *os.File) error {
	return file.Close()
}
