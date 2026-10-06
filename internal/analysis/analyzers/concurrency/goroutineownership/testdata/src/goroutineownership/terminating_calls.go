package goroutineownership

// A return reached only through a call that never returns does not skip the
// join: the result summary proves a project's fatal wrapper never returns.
// A wrapper that may return keeps the early return reachable.

import (
	"os"
	"sync"
)

func fatal(message string) {
	_ = message
	os.Exit(1)
}

func fatalIf(fail bool) {
	if fail {
		os.Exit(1)
	}
}

func joinedUnlessFatal(fail bool) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done() }()
	if fail {
		fatal("failed")
		return
	}
	wg.Wait()
}

func joinedUnlessMaybeFatal(fail, other bool) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done() }() // want "goroutine is not joined on every return path"
	if fail {
		fatalIf(other)
		return
	}
	wg.Wait()
}
