package helpers

// Equal branches describe one send, even though its summary retains both
// source positions. Source attribution must not multiply execution counts.
func equalBranchSend(yes bool) {
	ch := make(chan int)
	go func() {
		if yes {
			ch <- 1
		} else {
			ch <- 2
		}
	}()
	<-ch
}

// Folding each send in a two-send protocol still leaves an excess send.
func equalBranchExcess(yes bool) {
	ch := make(chan int)
	go func() {
		if yes {
			ch <- 1
			ch <- 2 // want "goroutine send can block"
		} else {
			ch <- 3
			ch <- 4 // want "goroutine send can block"
		}
	}()
	<-ch
}

func equalBranchBalancedPair(yes bool) {
	ch := make(chan int)
	go func() {
		if yes {
			ch <- 1
			ch <- 2
		} else {
			ch <- 3
			ch <- 4
		}
	}()
	<-ch
	<-ch
}

// Separate workers really do compete for the same receive. Each folded
// operation counts once, and every branch source keeps its diagnostic.
func equalBranchCompetingWorkers(yes bool) {
	ch := make(chan int)
	go func() {
		if yes {
			ch <- 1 // want "goroutine send can block"
		} else {
			ch <- 2 // want "goroutine send can block"
		}
	}()
	go func() {
		if yes {
			ch <- 3 // want "goroutine send can block"
		} else {
			ch <- 4 // want "goroutine send can block"
		}
	}()
	<-ch
}
