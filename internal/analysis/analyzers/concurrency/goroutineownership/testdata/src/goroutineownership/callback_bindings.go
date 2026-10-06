package goroutineownership

// A locally bound invoker exposes the callback's completion promise through
// the shared exact invocation engine. Joining that promise is still caller
// policy. An invoker that launches the callback again promises no synchronous
// completion; this check deliberately leaves that worker opaque.

func boundWorker(callback func(), invoke func(func())) {
	invoke(callback)
}

func joinedBoundWorker() {
	done := make(chan struct{})
	go boundWorker(func() { close(done) }, func(callback func()) { callback() })
	<-done
}

func opaqueBoundWorker() {
	done := make(chan struct{})
	go boundWorker(func() { close(done) }, func(callback func()) { go callback() })
}

func droppedBoundWorker() {
	done := make(chan struct{})
	go boundWorker(func() { close(done) }, func(callback func()) {})
}

func unjoinedBoundWorker(join bool) {
	done := make(chan struct{})
	go boundWorker(func() { close(done) }, func(callback func()) { callback() }) // want "goroutine is not joined on every return path"
	if join {
		<-done
	}
}
