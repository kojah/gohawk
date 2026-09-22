package channelprotocol

func capturedHelper() {
	results, done := make(chan int), make(chan struct{})
	go func() { wrapper(results, done) }()
	wait(done) // want "channel wait prevents the worker's preceding send from completing"
	drain(results)
}

func waitThenDrain(done <-chan struct{}, results <-chan int) {
	<-done
	<-results
}

func combinedHelper() {
	results, done := make(chan int), make(chan struct{})
	go worker(results, done)
	waitThenDrain(done, results) // want "channel wait prevents the worker's preceding send from completing"
}

func worker(results chan<- int, done chan<- struct{}) {
	results <- 42
	close(done)
}

func wrapper(results chan<- int, done chan<- struct{}) {
	worker(results, done)
}

func wait(done <-chan struct{}) { <-done }
func drain(results <-chan int) { <-results }

func composed() {
	results, done := make(chan int), make(chan struct{})
	go wrapper(results, done)
	wait(done) // want "channel wait prevents the worker's preceding send from completing"
	drain(results)
}

func direct() {
	results, done := make(chan int), make(chan struct{})
	go worker(results, done)
	<-done // want "channel wait prevents the worker's preceding send from completing"
	<-results
}

func throughField() {
	owner := struct{ result chan int }{make(chan int)}
	done := make(chan struct{})
	go wrapper(owner.result, done)
	<-done // want "channel wait prevents the worker's preceding send from completing"
	<-owner.result
}

func savedSnapshot() {
	owner := struct{ result chan int }{make(chan int)}
	saved := owner.result
	owner.result = make(chan int, 1)
	done := make(chan struct{})
	go worker(saved, done)
	<-done // want "channel wait prevents the worker's preceding send from completing"
	<-saved
}
