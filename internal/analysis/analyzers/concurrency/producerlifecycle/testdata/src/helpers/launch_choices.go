package helpers

// Only one worker starts; counting both branch launches invents a send.
func alternativeLaunches(yes bool) {
	ch := make(chan int)
	if yes {
		go func() { ch <- 1 }()
	} else {
		go func() { ch <- 2 }()
	}
	<-ch
}

// Each alternative can precede the common worker, but the alternatives cannot
// both run. Pairwise reachability against the common worker is insufficient.
func alternativesBeforeCommonWorker(yes bool) {
	ch := make(chan int)
	if yes {
		go func() { ch <- 1 }()
	} else {
		go func() { ch <- 2 }()
	}
	go func() { ch <- 3 }()
	<-ch
	<-ch
}

// Serial launches still compete for one receive.
func orderedLaunches() {
	ch := make(chan int)
	go func() { ch <- 1 }() // want "goroutine send can block"
	go func() { ch <- 2 }() // want "goroutine send can block"
	<-ch
}

// A dominating launch and its nested later worker do have a coexistence path.
func nestedOrderedLaunches(yes bool) {
	ch := make(chan int)
	go func() { ch <- 1 }() // want "goroutine send can block"
	if yes {
		go func() { ch <- 2 }() // want "goroutine send can block"
	}
	<-ch
}
