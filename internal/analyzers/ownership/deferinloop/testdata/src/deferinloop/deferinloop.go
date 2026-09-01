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

type resourceOwner struct {
	resource *os.File
	mutex    sync.Mutex
}

func accumulatedNestedResources(names []string) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		item := &resourceOwner{resource: file}
		defer item.resource.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
	}
	return nil
}

func accumulatedIndexedResources(names []string) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		files := []*os.File{file}
		defer files[0].Close() // want "deferred cleanup runs after the loop instead of after this iteration"
	}
	return nil
}

func accumulatedNestedLocks(items []int) {
	for range items {
		item := new(resourceOwner)
		item.mutex.Lock()
		defer item.mutex.Unlock() // want "deferred cleanup runs after the loop instead of after this iteration"
	}
}

func accumulatedIndexedLocks(items []int) {
	for range items {
		locks := []*sync.Mutex{new(sync.Mutex)}
		locks[0].Lock()
		defer locks[0].Unlock() // want "deferred cleanup runs after the loop instead of after this iteration"
	}
}

func cleanupOnTerminalMatch(names []string, wanted string) (*os.File, error) {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		if name == wanted {
			defer file.Close()
			return file, nil
		}
		_ = file.Close()
	}
	return nil, os.ErrNotExist
}
