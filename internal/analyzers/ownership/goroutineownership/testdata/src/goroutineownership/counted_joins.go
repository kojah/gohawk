package goroutineownership

import "sync"

// This file covers joins observed through receive loops, relay workers,
// receive helpers, and nested barriers. A receive loop is accepted whenever it
// follows the spawn; the analyzer does not compare loop counts, so a mismatch
// between spawn and receive counts is a known false negative rather than a
// diagnostic.

func joinedByCountedReceives() {
	done := make(chan bool)
	for range 10 {
		go func() { done <- true }()
	}
	for range 10 {
		<-done
	}
}

func joinedByMatchingDynamicCount(count int) {
	done := make(chan bool)
	for range count {
		go func() { done <- true }()
	}
	for range count {
		<-done
	}
}

func joinedByMatchingDynamicCountRepeated(count int) {
	for range 3 {
		done := make(chan bool)
		for range count {
			go func(value int) { done <- value > 0 }(count)
		}
		for range count {
			<-done
		}
		close(done)
	}
}

func returnsMatchingDynamicJoin(count int) func() {
	return func() {
		for range 3 {
			done := make(chan bool)
			for range count {
				go func(value int) { done <- value > 0 }(count)
			}
			for range count {
				<-done
			}
			close(done)
		}
	}
}

func joinedByMatchingSlice(items []int) {
	done := make(chan bool)
	for range items {
		go func() { done <- true }()
	}
	for range items {
		<-done
	}
}

func joinedByMatchingMap(items map[string]int) {
	done := make(chan bool)
	for range items {
		go func() { done <- true }()
	}
	for range len(items) {
		<-done
	}
}

func joinedThroughFiniteChannelMap() {
	first := make(chan struct{}, 1)
	second := make(chan struct{}, 1)
	go func() { first <- struct{}{} }()
	go func() { second <- struct{}{} }()
	for _, done := range map[string]<-chan struct{}{"first": first, "second": second} {
		<-done
	}
}

func eventually(check func() bool) { _ = check() }

func collectEventually(done <-chan bool, count int) {
	seen := 0
	eventually(func() bool {
		for {
			select {
			case <-done:
				seen++
				if seen == count {
					return true
				}
			default:
				return false
			}
		}
	})
}

func joinedByEventuallyCount(count int) {
	done := make(chan bool, count)
	for range count {
		go func() { done <- true }()
	}
	collectEventually(done, count)
}

func joinedBySliceCountdown(items []int) {
	done := make(chan bool)
	for range items {
		go func() { done <- true }()
	}
	for remaining := len(items); remaining > 0; remaining-- {
		<-done
	}
}

func joinTransferredToWaiter() {
	var group sync.WaitGroup
	group.Add(1)
	go func() { group.Done() }()
	done := make(chan struct{})
	go func() {
		group.Wait()
		close(done)
	}()
	<-done
}

func joinTransferredThroughRelay() {
	first := make(chan struct{})
	go func() { close(first) }()
	second := make(chan struct{})
	go func() {
		<-first
		close(second)
	}()
	<-second
}

func joinedThroughReceiveHelper() {
	done := make(chan bool)
	go func() { done <- true }()
	wait := func() { <-done }
	wait()
}

func receiveSignal(signal <-chan bool) {
	<-signal
}

func joinedThroughStaticReceiveHelper() {
	done := make(chan bool)
	go func() { done <- true }()
	receiveSignal(done)
}

func receiveGeneric[T any](signal <-chan T) T {
	return <-signal
}

func joinedThroughGenericReceiveHelper() {
	done := make(chan bool)
	go func() { done <- true }()
	_ = receiveGeneric(done)
}

func waitWithoutReceive(<-chan bool) {}

func joinWithoutReceive(<-chan bool) {}

func namedWaitAndJoinDoNotProveCompletion() {
	waitDone := make(chan bool)
	go func() { waitDone <- true }() // want "goroutine is not joined on every return path"
	waitWithoutReceive(waitDone)

	joinDone := make(chan bool)
	go func() { joinDone <- true }() // want "goroutine is not joined on every return path"
	joinWithoutReceive(joinDone)
}

type callbackOwner struct {
	run func()
}

func completionTransferredToCallback(register func(callbackOwner)) {
	done := make(chan struct{})
	go func() { close(done) }()
	register(callbackOwner{run: func() { <-done }})
}

type barrierTask struct {
	run func()
}

func completionTransferredThroughNestedWorkers() {
	var arrived sync.WaitGroup
	arrived.Add(2)
	both := make(chan struct{})
	go func() { arrived.Wait(); close(both) }()

	run := func(done chan<- struct{}) {
		task := barrierTask{run: func() {
			arrived.Done()
			<-both
		}}
		task.run()
		done <- struct{}{}
	}
	done := make(chan struct{}, 2)
	go run(done)
	go run(done)
	<-done
	<-done
}

func unrelatedNestedWorkerDoesNotJoin() {
	done := make(chan struct{})
	unrelated := make(chan struct{})
	go func() { close(done) }() // want "goroutine is not joined on every return path"
	run := func() {
		task := barrierTask{run: func() { <-unrelated }}
		task.run()
	}
	_ = run
	_ = done
}
