package goroutineownership

// A worker started under one guard and joined under the same guard is joined
// on every path the guard admits: the obligation walk remembers the branch
// it took and does not walk the other arm of the same stable guard later. A
// join under a different guard, or under a guard read from a cell that a
// hidden store could change, keeps the early return reachable.

import "sync"

type joinOptions struct{ wait bool }

func joinedUnderSameParameterGuard(enabled bool) {
	var wg sync.WaitGroup
	if enabled {
		wg.Add(1)
		go func() { defer wg.Done() }()
	}
	if !enabled {
		return
	}
	wg.Wait()
}

func joinedUnderDifferentParameterGuard(enabled, other bool) {
	var wg sync.WaitGroup
	if enabled {
		wg.Add(1)
		go func() { defer wg.Done() }() // want "goroutine is not joined on every return path"
	}
	if !other {
		return
	}
	wg.Wait()
}

func joinedUnderLoadedGuard(o *joinOptions) {
	var wg sync.WaitGroup
	if o.wait {
		wg.Add(1)
		go func() { defer wg.Done() }()
	}
	if !o.wait {
		return
	}
	wg.Wait()
}
