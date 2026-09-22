package channelprotocol

import "sync"

// A scalar result does not itself establish purity. Opaque calls, callbacks,
// global effects and possibly panicking expressions stay unknown.
func opaqueCompute() int

func groupOpaqueWork() {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	go func() { defer group.Done(); results <- opaqueCompute() }()
	group.Wait()
	<-results
}

var computed int
func mutatingCompute() int { computed = 1; return computed }

func groupMutatingWork() {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	go func() { defer group.Done(); results <- mutatingCompute() }()
	group.Wait()
	<-results
}

func groupCallbackWork(compute func() int) {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	go func() { defer group.Done(); results <- compute() }()
	group.Wait()
	<-results
}

func divide(n int) int { return 1 / n }

func groupPossiblyPanickingWork(n int) {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	go func() { defer group.Done(); results <- divide(n) }()
	group.Wait()
	<-results
}

func scalarCompute(n int) int { return n*2 + 1 }

func groupScalarWork(n int) {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	go func(n int) { defer group.Done(); results <- scalarCompute(n) }(n)
	group.Wait() // want "WaitGroup wait prevents the worker's preceding send from completing"
	<-results
}
