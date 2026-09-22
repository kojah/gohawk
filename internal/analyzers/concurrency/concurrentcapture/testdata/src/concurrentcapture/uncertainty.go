package concurrentcapture

import "sync"

// Channel regions whose capacity is unknown are deliberately not proven safe.
// The syntax check cannot establish competing writers inside those protocols.
// Similarly, range-key guards may select one writer, but their cardinality is
// not evaluated here. Constant/shared guards remain diagnostic controls.

func channelRegion(items []int) {
	gate := make(chan struct{}, 1)
	var group sync.WaitGroup
	var result error
	for range items {
		group.Add(1)
		go func() {
			defer group.Done()
			gate <- struct{}{}
			result = work()
			<-gate
		}()
	}
	group.Wait()
	_ = result
}

func unrelatedChannel(items []int, gate, other chan struct{}) {
	var result error
	for range items {
		go func() {
			gate <- struct{}{}
			result = work() // want "captured local result is mutated"
			<-other
		}()
	}
	_ = result
}

func endedChannelRegion(items []int, gate chan struct{}) {
	var result error
	for range items {
		go func() {
			gate <- struct{}{}
			<-gate
			result = work() // want "captured local result is mutated"
		}()
	}
	_ = result
}

func partitionedWorkers() {
	var group sync.WaitGroup
	var first, second error
	for i := range []int{1, 2} {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			if index%2 == 0 {
				first = work()
			} else {
				second = work()
			}
		}(i)
	}
	group.Wait()
	_, _ = first, second
}

func sharedWorkerCondition(items []int, condition bool) {
	var result error
	for range items {
		go func(flag bool) {
			if flag {
				result = work() // want "captured local result is mutated"
			}
		}(condition)
	}
	_ = result
}

func ordinaryConditionalWrite(items []int, condition bool) {
	var result error
	for range items {
		go func() {
			if condition {
				result = work() // want "captured local result is mutated"
			}
		}()
	}
	_ = result
}
