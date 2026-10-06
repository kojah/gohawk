package goroutineownership

import "sync"

// Counts and collection-wide coverage deliberately remain unknown. An early
// Done inside a loop is not diagnosed: the counter may count items, not workers.
// An early Done outside a loop is likewise unknown: it may signal readiness.
func countedItems(items []func()) {
	var group sync.WaitGroup
	group.Add(len(items))
	go func() {
		for _, item := range items {
			item()
			group.Done()
		}
	}()
	group.Wait()
}

func stripedResults(items []int, workers int) {
	channels := make([]chan int, workers)
	for i := 0; i < workers; i++ {
		channels[i] = make(chan int, 1)
		go func(i int) {
			for j := i; j < len(items); j += workers {
				channels[i] <- items[j]
			}
		}(i)
	}
	for i := range items {
		<-channels[i%workers]
	}
}

func unrelatedReceive() {
	done := make(chan int)
	other := make(chan int)
	go func() { done <- 1 }() // want "goroutine is not joined on every return path"
	<-other
}

func registryGroup(registry *sync.Map, task func()) {
	value, _ := registry.Load("group")
	if group, ok := value.(*sync.WaitGroup); ok {
		group.Add(1)
		go func() {
			task()
			group.Done()
		}()
	}
}

func localGroupNotWaited(task func()) {
	group := new(sync.WaitGroup)
	group.Add(1)
	go func() { // want "goroutine is not joined on every return path"
		task()
		group.Done()
	}()
}

type workerGroupOwner struct{ group *sync.WaitGroup }

func (owner *workerGroupOwner) Wait() { owner.group.Wait() }

func deferredWorkerGroup(group *sync.WaitGroup, output chan int) {
	defer func() {
		if output != nil {
			close(output)
		}
		group.Done()
	}()
}

func returnedDeferredGroup() *workerGroupOwner {
	owner := &workerGroupOwner{group: new(sync.WaitGroup)}
	owner.group.Add(1)
	go deferredWorkerGroup(owner.group, make(chan int))
	return owner
}

func unrelatedReturnedDeferredGroup() *workerGroupOwner {
	owner := &workerGroupOwner{group: new(sync.WaitGroup)}
	other := &workerGroupOwner{group: new(sync.WaitGroup)}
	owner.group.Add(1)
	go deferredWorkerGroup(owner.group, make(chan int)) // want "goroutine is not joined on every return path"
	return other
}
