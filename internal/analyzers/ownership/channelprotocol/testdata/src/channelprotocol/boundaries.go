package channelprotocol

// These cases exercise completeness of the participant/effect model, not just
// the order of the two operations that would otherwise resemble a cycle.

func launchReceiver(results chan int) { go func() { <-results }() }

func hiddenParticipant() {
	results, done := make(chan int), make(chan struct{})
	launchReceiver(results)
	go worker(results, done)
	<-done
	<-results
}

func deferredSignal() {
	results, done := make(chan int), make(chan struct{})
	go func() {
		defer close(done)
		results <- 42
	}()
	<-done
	<-results
}

func sameChannel() {
	channel := make(chan int)
	go func() {
		channel <- 42
		close(channel)
	}()
	<-channel
	<-channel
}

func changedField() {
	owner := struct{ result chan int }{make(chan int)}
	done := make(chan struct{})
	go func() {
		owner.result <- 42
		close(done)
	}()
	owner.result = make(chan int, 1)
	<-done
	<-owner.result
}

func referencePayload() {
	results, done := make(chan chan int), make(chan struct{})
	go func() {
		results <- nil
		close(done)
	}()
	<-done
	<-results
}

func twice(ch chan int) { ch <- 1; ch <- 2 }
func four(ch chan int) { twice(ch); twice(ch) }
func eight(ch chan int) { four(ch); four(ch) }
func sixteen(ch chan int) { eight(ch); eight(ch) }
func thirtyTwo(ch chan int) { sixteen(ch); sixteen(ch) }
func overLimitWorker(ch chan int, done chan struct{}) { thirtyTwo(ch); close(done) }

func overLimit() {
	results, done := make(chan int), make(chan struct{})
	go overLimitWorker(results, done)
	<-done
	<-results
}
