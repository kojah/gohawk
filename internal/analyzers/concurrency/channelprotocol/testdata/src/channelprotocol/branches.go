package channelprotocol

import "effecthelpers"

func equivalentWorker(result, done chan int, flag bool) {
	if flag {
		result <- 1
	} else {
		result <- 2
	}
	close(done)
}

func branchCycle(flag bool) {
	result, done := make(chan int), make(chan int)
	go equivalentWorker(result, done, flag)
	<-done // want "channel wait prevents the worker's preceding send"
	<-result
}

func importedBranchCycle(flag bool) {
	result, done := make(chan int), make(chan int)
	go effecthelpers.BranchWorker(result, done, flag)
	<-done // want "channel wait prevents the worker's preceding send"
	<-result
}

func branchSafe(flag bool) {
	result, done := make(chan int), make(chan int)
	go equivalentWorker(result, done, flag)
	<-result
	<-done
}

func optionalWorker(result, done chan int, flag bool) {
	if flag {
		result <- 1
	}
	close(done)
}

func optionalUnknown(flag bool) {
	result, done := make(chan int), make(chan int)
	go optionalWorker(result, done, flag)
	<-done
	<-result
}
