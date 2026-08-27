package deferinloop

import (
	"os"
	"sync"
)

func accumulatedFiles(names []string) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
	}
	return nil
}

func accumulatedLocks(items []int) {
	var mutex sync.RWMutex
	for range items {
		mutex.RLock()
		defer mutex.RUnlock() // want "deferred cleanup runs after the loop instead of after this iteration"
	}
}

func scopedCleanup(names []string) error {
	for _, name := range names {
		if err := useFile(name); err != nil {
			return err
		}
	}
	return nil
}

func useFile(name string) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

func harmlessTracing(items []int) {
	for range items {
		defer recordIteration()
	}
}

func recordIteration() {}
