package deferinloop

import (
	"database/sql"
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

type namedLikeCleanup struct{}

func (*namedLikeCleanup) Close() {}

func misleadingCleanupName(items []int) {
	for range items {
		value := new(namedLikeCleanup)
		defer value.Close()
	}
}

func closeFile(file *os.File) { _ = file.Close() }

func filesSettledBeforeNextIteration(names []string) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		closeFile(file)
	}
	return nil
}

func filesOnlySometimesSettled(names []string, settle bool) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
		if settle {
			_ = file.Close()
		}
	}
	return nil
}

func transactionsCommittedBeforeNextIteration(db *sql.DB, items []int) error {
	for range items {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

var retainedFiles []*os.File

func retainFile(file *os.File) { retainedFiles = append(retainedFiles, file) }

func filesTransferredBeforeNextIteration(names []string) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		retainFile(file)
	}
	return nil
}

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
