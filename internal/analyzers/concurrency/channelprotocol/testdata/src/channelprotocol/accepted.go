package channelprotocol

// Accepted boundaries come first: spare capacity, servicing results before
// completion, additional participants, alternative control flow, and effects
// outside the visible scope must never become a waiting-cycle proof.

func buffered() {
	results, done := make(chan int, 1), make(chan struct{})
	go worker(results, done)
	<-done
	<-results
}

func receiveFirst() {
	results, done := make(chan int), make(chan struct{})
	go worker(results, done)
	<-results
	<-done
}

func signalFirstWorker(results chan<- int, done chan<- struct{}) {
	close(done)
	results <- 42
}

func signalFirst() {
	results, done := make(chan int), make(chan struct{})
	go signalFirstWorker(results, done)
	<-done
	<-results
}

func extraReceiver() {
	results, done := make(chan int), make(chan struct{})
	go worker(results, done)
	go func() { <-results }()
	<-done
}

func externalChannels(results chan int, done chan struct{}) {
	go worker(results, done)
	<-done
	<-results
}

var published chan int

func escapes() {
	results, done := make(chan int), make(chan struct{})
	published = results
	go worker(results, done)
	<-done
	<-results
}

func opaque(chan int)

func opaqueParticipant() {
	results, done := make(chan int), make(chan struct{})
	opaque(results)
	go worker(results, done)
	<-done
	<-results
}

func conditionalWorker(results chan<- int, done chan<- struct{}, send bool) {
	if send {
		results <- 42
	}
	close(done)
}

func conditional(send bool) {
	results, done := make(chan int), make(chan struct{})
	go conditionalWorker(results, done, send)
	<-done
}

func cancellable(cancel <-chan struct{}) {
	results, done := make(chan int), make(chan struct{})
	go func() {
		select {
		case results <- 42:
		case <-cancel:
		}
		close(done)
	}()
	<-done
}

func dynamicCapacity(n int) {
	results, done := make(chan int, n), make(chan struct{})
	go worker(results, done)
	<-done
	<-results
}

func recursiveWorker(results chan<- int, done chan<- struct{}) {
	recursiveWorker(results, done)
	results <- 42
	close(done)
}

func recursive() {
	results, done := make(chan int), make(chan struct{})
	go recursiveWorker(results, done)
	<-done
	<-results
}

func panicWorker(results chan<- int, done chan<- struct{}) {
	panic("never reaches the protocol")
}

func abnormalExit() {
	results, done := make(chan int), make(chan struct{})
	go panicWorker(results, done)
	<-done
	<-results
}

func replacedCapture() {
	results, done := make(chan int), make(chan struct{})
	go func() {
		results <- 42
		close(done)
	}()
	results = make(chan int, 1)
	<-done
	<-results
}

// Repeated launches and their iteration-dependent channel identities remain
// outside this proof; they are not evidence of a fixed two-party protocol.
func repeated() {
	for i := 0; i < 2; i++ {
		results, done := make(chan int), make(chan struct{})
		go worker(results, done)
		<-done
		<-results
	}
}
