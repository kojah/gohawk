package causal

// This file covers test-only causal joins where one worker ends blocking work
// owned by another goroutine. The lifecycle action must use the shared owner.

type blocker struct {
	stopped chan struct{}
}

func newBlocker() *blocker {
	return &blocker{stopped: make(chan struct{})}
}

func serve(value *blocker) {
	<-value.stopped
}

func finish(value *blocker) {
	close(value.stopped)
}

func observe(*blocker) {}

func waitFor(value *blocker) {
	<-value.stopped
}

func lifecycleActionEndsBlockingWorker() {
	value := newBlocker()
	go func() { close(value.stopped) }()
	waitFor(value)
}

func unrelatedLifecycleActionDoesNotJoinWorker() {
	value := newBlocker()
	other := newBlocker()
	go func() { // want "goroutine is not joined on every return path"
		observe(value)
		close(other.stopped)
	}()
	waitFor(value)
}

func joinedControllerEndsBlockingWorker() {
	value := newBlocker()
	done := make(chan struct{})
	go serve(value)
	go func() {
		finish(value)
		close(done)
	}()
	<-done
}

func unrelatedJoinedControllerDoesNotOwnWorker() {
	value := newBlocker()
	other := newBlocker()
	done := make(chan struct{})
	go serve(value) // want "goroutine is not joined on every return path"
	go func() {
		observe(value)
		finish(other)
		close(done)
	}()
	<-done
}
