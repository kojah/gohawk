package channelcycle

import "channelcyclehelper"

func channelWithExternalPartner(ch chan int) {
	ch <- 1
}

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

func importedLaunchCreatesChild() {
	a := make(chan int)
	b := make(chan int)
	channelcyclehelper.Launch(b, a)
	a <- 1 // want "two goroutines wait on each other's later channel operation"
	<-b
}

func forwardedLaunchCreatesChild() {
	a := make(chan int)
	b := make(chan int)
	channelcyclehelper.Forward(b, a)
	a <- 1 // want "two goroutines wait on each other's later channel operation"
	<-b
}

func optionalHelperLaunchIsUnknown(run bool) {
	a := make(chan int)
	b := make(chan int)
	channelcyclehelper.MaybeLaunch(b, a, run)
	a <- 1
	<-b
}

func loopedHelperLaunchIsUnknown(count int) {
	a := make(chan int)
	b := make(chan int)
	channelcyclehelper.LaunchInLoop(b, a, count)
	a <- 1
	<-b
}

func separateCallsCreateSeparateChildren() {
	a := make(chan int)
	b := make(chan int)
	channelcyclehelper.Launch(b, a)
	channelcyclehelper.Launch(b, a)
	a <- 1 // want "two goroutines wait on each other's later channel operation"
	<-b
}
