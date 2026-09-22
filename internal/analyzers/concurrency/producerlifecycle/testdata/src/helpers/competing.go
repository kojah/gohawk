package helpers

func competingReceiver(opaque func()) {
	ch := make(chan int)
	go func() { ch <- 1; ch <- 2 }()
	go func() { opaque(); <-ch }()
	<-ch
}

func escapedCompetingChannel(opaque func(chan int)) {
	ch := make(chan int)
	go func() { ch <- 1; ch <- 2 }()
	go func() { opaque(ch) }()
	<-ch
}
