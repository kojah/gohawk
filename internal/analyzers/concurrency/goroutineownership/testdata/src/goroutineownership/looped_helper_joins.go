package goroutineownership

// A visible helper that joins every worker it was handed inside a loop is an
// uncertainty boundary: the join is not on every return of the helper, and
// which worker an iteration joins is decided by iteration. The caller is not
// reported. A helper that loops without joining keeps the obligation open.

func drainAll(dones []chan struct{}) {
	for _, done := range dones {
		<-done
	}
}

func inspectAll(dones []chan struct{}) {
	for _, done := range dones {
		_ = done
	}
}

func workerJoinedByLoopingHelper() {
	done := make(chan struct{})
	go func() { close(done) }()
	drainAll([]chan struct{}{done})
}

func workerInspectedByLoopingHelper() {
	done := make(chan struct{})
	go func() { close(done) }() // want "goroutine is not joined on every return path"
	inspectAll([]chan struct{}{done})
}
