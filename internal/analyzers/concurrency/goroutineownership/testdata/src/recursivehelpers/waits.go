package recursivehelpers

import "sync"

// Unsupported recursive forwarding supplies unknown completion evidence.
// This can miss real omissions in recursive helpers; it never proves a join
// by guessing how often the helper runs. An explicit final Wait is exact.
func recursiveWait(group *sync.WaitGroup, n int) {
	if n > 0 {
		recursiveWait(group, n-1)
		return
	}
	group.Wait()
}

func recursiveObserved() {
	group := new(sync.WaitGroup)
	group.Add(1)
	go func() { defer group.Done() }()
	recursiveWait(group, 1)
}

func mutualWaitA(group *sync.WaitGroup, n int) {
	if n > 0 {
		mutualWaitB(group, n-1)
		return
	}
	group.Wait()
}

func mutualWaitB(group *sync.WaitGroup, n int) { mutualWaitA(group, n) }

func mutualObserved() {
	group := new(sync.WaitGroup)
	group.Add(1)
	go func() { defer group.Done() }()
	mutualWaitA(group, 1)
}

func recursiveThenWait(group *sync.WaitGroup, n int) {
	if n > 0 {
		recursiveThenWait(group, n-1)
	}
	group.Wait()
}

func exactAfterRecursive() {
	group := new(sync.WaitGroup)
	group.Add(1)
	go func() { defer group.Done() }()
	recursiveThenWait(group, 1)
}

func unrelatedRecursive() {
	group := new(sync.WaitGroup)
	group.Add(1)
	go func() { defer group.Done() }() // want "goroutine is not joined on every return path"
	recursiveWait(new(sync.WaitGroup), 1)
}
