package channelprotocol

//gohawk:example flagged Waiting before receiving the worker's result
func waitBeforeReceive() {
	results, done := make(chan int), make(chan struct{})
	go func() {
		results <- 42
		close(done)
	}()
	<-done // want "channel wait prevents the worker's preceding send from completing"
	<-results
}
//gohawk:example end

//gohawk:example ok
func receiveBeforeWait() {
	results, done := make(chan int), make(chan struct{})
	go func() {
		results <- 42
		close(done)
	}()
	<-results
	<-done
}
//gohawk:example end
