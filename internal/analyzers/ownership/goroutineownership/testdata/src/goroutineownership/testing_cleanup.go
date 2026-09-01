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
	go func() { worker.run() }() // want "goroutine is not joined on every return path"
	t.Cleanup(func() { other.Stop() })
}

func conditionallyStoppedByTestCleanup(t *testing.T, stop bool) {
	owner := &lifecycleOwner{}
	go func() { owner.run() }() // want "goroutine is not joined on every return path"
	t.Cleanup(func() {
		if stop {
			owner.Stop()
		}
	})
}

func unrelatedTestCleanup(t *testing.T) {
	owner := &lifecycleOwner{}
	go func() { owner.run() }() // want "goroutine is not joined on every return path"
	t.Cleanup(func() { owner.Observe() })
}

func cleanupCannotStopIndependentBlock(t *testing.T) {
	owner := &lifecycleOwner{}
	go func() { // want "goroutine is not joined on every return path"
		owner.Observe()
		select {}
	}()
	t.Cleanup(func() { owner.Stop() })
}

func waitGroupJoinedByTestCleanup(t *testing.T) {
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }()
	t.Cleanup(func() { group.Wait() })
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
