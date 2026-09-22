package channelprotocol

// A defer runs at the return of its own function, not when it is registered
// and not at the return of its caller. Unknown or panicking defers cannot be
// discarded just because a completion defer also exists.
func deferredReceiveFirst() {
	results, done := make(chan int), make(chan struct{})
	go deferredWorker(results, done)
	<-results
	<-done
}

func deferredBuffered() {
	results, done := make(chan int, 1), make(chan struct{})
	go deferredWorker(results, done)
	<-done
	<-results
}

func closeAtReturn(done chan struct{}) { defer close(done) }

func helperReturnsBeforeSend() {
	results, done := make(chan int), make(chan struct{})
	go func() { closeAtReturn(done); results <- 1 }()
	<-done
	<-results
}

func deferredUnknown() {
	results, done := make(chan int), make(chan struct{})
	go func() { defer close(done); defer opaque(results); results <- 1 }()
	<-done
	<-results
}

func deferredPanic() {
	results, done := make(chan int), make(chan struct{})
	go func() { defer close(done); panic("no result") }()
	<-done
	<-results
}

func closeAndSend(done chan struct{}, results chan int) { close(done); results <- 1 }

func deferredMixedEffects() {
	results, done := make(chan int), make(chan struct{})
	go func() { defer closeAndSend(done, results) }()
	<-done
	<-results
}

func deferredWorker(results chan<- int, done chan struct{}) {
	defer close(done)
	results <- 1
}

func deferredCycle() {
	results, done := make(chan int), make(chan struct{})
	go deferredWorker(results, done)
	<-done // want "channel wait prevents the worker's preceding send from completing"
	<-results
}

func deferredClosureCycle() {
	results, done := make(chan int), make(chan struct{})
	go func() { defer func() { close(done) }(); results <- 1 }()
	<-done // want "channel wait prevents the worker's preceding send from completing"
	<-results
}

func deferredOrderWorker(results chan int, first, second chan struct{}) {
	defer close(first)
	defer close(second)
	results <- 1
}
