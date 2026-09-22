package channelprotocol

import "sync"

// Accepted boundaries: zero counts, early Done, independent groups, extra
// participants, mutable group storage, opaque work and dynamic counts must
// never be mistaken for the exact one-worker obligation below.
func groupReceiveFirst() {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	go groupWorker(&group, results)
	<-results
	group.Wait()
}

func groupBuffered() {
	var group sync.WaitGroup
	results := make(chan int, 1)
	group.Add(1)
	go groupWorker(&group, results)
	group.Wait()
	<-results
}

func groupEarlyDone() {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	go func() { group.Done(); results <- 1 }()
	group.Wait()
	<-results
}

func groupZeroCount() {
	var group sync.WaitGroup
	results := make(chan int)
	go groupWorker(&group, results)
	group.Wait()
	<-results
}

func differentGroup() {
	var first, second sync.WaitGroup
	results := make(chan int)
	first.Add(1)
	go groupWorker(&first, results)
	second.Wait()
	<-results
}

func dynamicGroupCount(n int) {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(n)
	go groupWorker(&group, results)
	group.Wait()
	<-results
}

func externalGroup(group *sync.WaitGroup) {
	results := make(chan int)
	group.Add(1)
	go groupWorker(group, results)
	group.Wait()
	<-results
}

func groupOtherReceiver() {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	go groupWorker(&group, results)
	go func() { <-results }()
	group.Wait()
}

func resetGroup() {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	group = sync.WaitGroup{}
	go groupWorker(&group, results)
	group.Wait()
	<-results
}

func lateAdd() {
	var group sync.WaitGroup
	results := make(chan int)
	go func() { group.Add(1); defer group.Done(); results <- 1 }()
	group.Wait()
	<-results
}

type pretendGroup struct{}
func (*pretendGroup) Add(int) {}
func (*pretendGroup) Done() {}
func (*pretendGroup) Wait() {}

func misleadingGroupNames() {
	var group pretendGroup
	results := make(chan int)
	group.Add(1)
	go func() { defer group.Done(); results <- 1 }()
	group.Wait()
	<-results
}

func groupWorker(group *sync.WaitGroup, results chan<- int) {
	defer group.Done()
	results <- 1
}

func groupDirectWorker(group *sync.WaitGroup, results chan<- int) {
	results <- 1
	group.Done()
}

func groupWrapper(group *sync.WaitGroup, results chan<- int) { groupWorker(group, results) }
func groupWait(group *sync.WaitGroup) { group.Wait() }

func groupCycle() {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	go groupWrapper(&group, results)
	groupWait(&group) // want "wait prevents the worker's preceding send from completing"
	<-results
}

func directGroupCycle() {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	go groupDirectWorker(&group, results)
	group.Wait() // want "wait prevents the worker's preceding send from completing"
	<-results
}

func capturedGroupCycle() {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	go func() { defer group.Done(); results <- 1 }()
	group.Wait() // want "wait prevents the worker's preceding send from completing"
	<-results
}

func replacedGroupPointer() {
	group := new(sync.WaitGroup)
	results := make(chan int)
	group.Add(1)
	go func() { defer group.Done(); results <- 1 }()
	group = new(sync.WaitGroup)
	group.Wait()
	<-results
}

func groupCountSettled() {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(1)
	group.Done()
	go groupWorker(&group, results)
	group.Wait()
	<-results
}

func groupLargerCount() {
	var group sync.WaitGroup
	results := make(chan int)
	group.Add(2)
	go groupWorker(&group, results)
	group.Wait()
	<-results
}
