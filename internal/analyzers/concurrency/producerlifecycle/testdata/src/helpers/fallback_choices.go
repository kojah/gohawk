package helpers

// The callback makes the protocol summary incomplete. Its direct sends still
// cannot be added across mutually exclusive branches before their common send.
func fallbackBranchBeforeCommon(yes bool, opaque func()) {
	ch := make(chan int)
	go func() {
		opaque()
		if yes {
			ch <- 1
		} else {
			ch <- 2
		}
		ch <- 3 // count-unknown
	}()
	<-ch
	<-ch
}

// Branch alternatives in another worker also cannot inflate the total serving
// this worker's send. Three receives balance the three possible sends.
func fallbackCompetingBranchWorker(yes bool, opaque func()) {
	ch := make(chan int)
	go func() { ch <- 0 }() // count-unknown
	go func() {
		opaque()
		if yes {
			ch <- 1
		} else {
			ch <- 2
		}
		ch <- 3 // count-unknown
	}()
	<-ch
	<-ch
	<-ch
}

func fallbackOrderedExcess(opaque func()) {
	ch := make(chan int)
	go func() {
		opaque()
		ch <- 1
		ch <- 2 // want "goroutine send can block"
	}()
	<-ch
}

func fallbackNestedOrderedExcess(yes bool, opaque func()) {
	ch := make(chan int)
	go func() {
		opaque()
		ch <- 1
		if yes {
			ch <- 2 // want "goroutine send can block"
		}
	}()
	<-ch
}
