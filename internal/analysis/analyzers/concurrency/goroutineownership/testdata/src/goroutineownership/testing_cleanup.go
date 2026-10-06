package goroutineownership

import (
	"sync"
	"testing"
)

func (*lifecycleOwner) Observe() {}

func stoppedByTestCleanup(t *testing.T) {
	owner := &lifecycleOwner{}
	go func() { owner.run() }()
	t.Cleanup(func() { owner.Stop() })
}

func differentOwnerStoppedByTestCleanup(t *testing.T) {
	worker := &lifecycleOwner{}
	other := &lifecycleOwner{}
	go func() { worker.run() }()
	t.Cleanup(func() { other.Stop() })
}

func conditionallyStoppedByTestCleanup(t *testing.T, stop bool) {
	owner := &lifecycleOwner{}
	go func() { owner.run() }()
	t.Cleanup(func() {
		if stop {
			owner.Stop()
		}
	})
}

func unrelatedTestCleanup(t *testing.T) {
	owner := &lifecycleOwner{}
	go func() { owner.run() }()
	t.Cleanup(func() { owner.Observe() })
}

func waitGroupJoinedByTestCleanup(t *testing.T) {
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }()
	t.Cleanup(func() { group.Wait() })
}

func waitGroupCleanupBeforeSpawn(t *testing.T) {
	var group sync.WaitGroup
	group.Add(1)
	t.Cleanup(group.Wait)
	go func() { defer group.Done() }()
}

func closureCleanupBeforeSpawn(t *testing.T) {
	var group sync.WaitGroup
	group.Add(1)
	t.Cleanup(func() { group.Wait() })
	go func() { defer group.Done() }()
}

func immediateWaitBeforeSpawn() {
	var group sync.WaitGroup
	group.Wait()
	group.Add(1)
	go func() { defer group.Done() }() // want "goroutine is not joined on every return path"
}

func wrongGroupCleanupBeforeSpawn(t *testing.T) {
	var group, other sync.WaitGroup
	group.Add(1)
	t.Cleanup(other.Wait)
	go func() { defer group.Done() }() // want "goroutine is not joined on every return path"
}

func differentWaitGroupInTestCleanup(t *testing.T) {
	var worker, other sync.WaitGroup
	worker.Add(1)
	go func() { defer worker.Done() }() // want "goroutine is not joined on every return path"
	t.Cleanup(func() { other.Wait() })
}

func conditionalWaitGroupInTestCleanup(t *testing.T, wait bool) {
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }() // want "goroutine is not joined on every return path"
	t.Cleanup(func() {
		if wait {
			group.Wait()
		}
	})
}
