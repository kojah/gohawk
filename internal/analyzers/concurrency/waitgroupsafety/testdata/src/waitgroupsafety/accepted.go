package waitgroupsafety

import "sync"

// Unknown starting counts, participants, effects and divergent paths must not
// become evidence of underflow. These deliberately include unmodeled defects.
func borrowed(group *sync.WaitGroup) { group.Done() }

func balanced() {
	var group sync.WaitGroup
	group.Add(1)
	group.Done()
	group.Wait()
}

func distinctGroups() {
	var first, second sync.WaitGroup
	first.Add(1)
	second.Add(1)
	first.Done()
	second.Done()
}

func deferredCompletion() {
	var group sync.WaitGroup
	group.Add(1)
	defer group.Done()
}

func reused() {
	var group sync.WaitGroup
	group.Add(1)
	group.Done()
	group.Wait()
	group.Add(1)
	group.Done()
}

func unknownCount(count int) {
	var group sync.WaitGroup
	group.Add(count)
	group.Done()
}

func blockedBeforeDone() {
	var group sync.WaitGroup
	group.Add(1)
	group.Wait()
	group.Done()
	group.Done()
}

func opaque(callback func(*sync.WaitGroup)) {
	var group sync.WaitGroup
	callback(&group)
	group.Done()
}

func twoWorkers() {
	var group sync.WaitGroup
	group.Add(1)
	group.Add(1)
	go group.Done()
	go group.Done()
	group.Wait()
}

func conditional(add bool) {
	var group sync.WaitGroup
	if add {
		group.Add(1)
	}
	group.Done()
}

func workerBalanced() {
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }()
	group.Wait()
}

func callerAlsoAdds() {
	var group sync.WaitGroup
	group.Add(1)
	go func() { group.Done(); group.Done() }()
	group.Add(1)
	group.Wait()
}

func reset() {
	var group sync.WaitGroup
	group.Add(1)
	group = sync.WaitGroup{}
	group.Done()
}

type lookalike struct{}

func (*lookalike) Done() {}
func misleading()        { var group lookalike; group.Done(); group.Done() }
