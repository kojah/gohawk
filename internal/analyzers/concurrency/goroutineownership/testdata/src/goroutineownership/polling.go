package goroutineownership

// This file covers completion observed indirectly through a polling predicate.

func isClosed(signal <-chan struct{}) bool {
	select {
	case <-signal:
		return true
	default:
		return false
	}
}

func requireEventually(predicate func() bool) {
	for !predicate() {
	}
}

func joinedByPollingObservation() {
	done := make(chan struct{})
	go func() { close(done) }()
	requireEventually(func() bool { return isClosed(done) })
}
