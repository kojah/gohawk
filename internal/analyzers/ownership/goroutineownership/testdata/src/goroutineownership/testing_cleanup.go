package goroutineownership

import "testing"

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
