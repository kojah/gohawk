package channelcycle

func bothSendFirst() {
	a := make(chan int)
	b := make(chan int)
	go func() {
		b <- 1
		<-a
	}()
	a <- 1 // want "two goroutines wait on each other's later channel operation"
	<-b
}

func bothReceiveFirst() {
	a := make(chan int)
	b := make(chan int)
	go func() {
		<-b
		a <- 1
	}()
	<-a // want "two goroutines wait on each other's later channel operation"
	b <- 1
}

func unrelatedChildStillBlocks() {
	a := make(chan int)
	b := make(chan int)
	go func() {}()
	go func() {
		b <- 1
		<-a
	}()
	a <- 1 // want "two goroutines wait on each other's later channel operation"
	<-b
}

func bufferedChannelCanAdvance() {
	a := make(chan int, 1)
	b := make(chan int)
	go func() {
		b <- 1
		<-a
	}()
	a <- 1
	<-b
}

func childCanSupplyFirstReceive() {
	a := make(chan int)
	b := make(chan int)
	go func() {
		a <- 1
		<-b
	}()
	<-a
	b <- 1
}

func anotherChildCanSupplyFirstReceive() {
	a := make(chan int)
	b := make(chan int)
	go func() {
		<-b
		a <- 1
	}()
	go func() { a <- 2 }()
	<-a
	b <- 1
}

func suppliedChannelMayHavePartner(a chan int) {
	b := make(chan int)
	go func() {
		b <- 1
		<-a
	}()
	a <- 1
	<-b
}

func cancellationSelectCanExit(cancel <-chan struct{}) {
	a := make(chan int)
	b := make(chan int)
	go func() {
		select {
		case b <- 1:
		case <-cancel:
			return
		}
		<-a
	}()
	a <- 1
	<-b
}

func equivalentSelectArmsCannotUnblock() {
	a := make(chan int)
	b := make(chan int)
	go func() {
		select {
		case b <- 1:
		case b <- 2:
		}
		<-a
	}()
	a <- 1 // want "two goroutines wait on each other's later channel operation"
	<-b
}
