package goroutineownership

// This file covers completion ownership transferred through returned closures,
// aggregates, and mutable fields, including incomplete transfer counterexamples.

func callerOwnsCompletion(done chan<- bool) {
	go func() { done <- true }()
}

type returnedWorker struct {
	wait func()
}

func returnedClosureOwnsCompletion() returnedWorker {
	done := make(chan bool)
	go func() { done <- true }()
	wait := func() { <-done }
	return returnedWorker{wait: wait}
}

type returnedLifecycleAggregate struct {
	worker *lifecycleOwner
}

func returnedAggregateOwnsSpawnedLifecycle() *returnedLifecycleAggregate {
	worker := &lifecycleOwner{}
	result := &returnedLifecycleAggregate{worker: worker}
	go worker.run()
	return result
}

func localAggregateDoesNotOwnSpawnedLifecycle() {
	worker := &lifecycleOwner{}
	result := &returnedLifecycleAggregate{worker: worker}
	go worker.run() // want "goroutine is not joined on every return path"
	_ = result
}

func unrelatedAggregateDoesNotOwnSpawnedLifecycle() *returnedLifecycleAggregate {
	worker := &lifecycleOwner{}
	local := &returnedLifecycleAggregate{worker: worker}
	result := &returnedLifecycleAggregate{}
	go worker.run() // want "goroutine is not joined on every return path"
	_ = local
	return result
}

func conditionalAggregateDoesNotOwnEveryReturn(useOwner bool) *returnedLifecycleAggregate {
	worker := &lifecycleOwner{}
	result := &returnedLifecycleAggregate{worker: worker}
	other := &returnedLifecycleAggregate{}
	go worker.run() // want "goroutine is not joined on every return path"
	if useOwner {
		return result
	}
	return other
}

func conditionalFieldStoreDoesNotOwnSpawnedLifecycle(install bool) *returnedLifecycleAggregate {
	worker := &lifecycleOwner{}
	result := &returnedLifecycleAggregate{}
	if install {
		result.worker = worker
	}
	go worker.run() // want "goroutine is not joined on every return path"
	return result
}

func replacedFieldDoesNotOwnSpawnedLifecycle() *returnedLifecycleAggregate {
	worker := &lifecycleOwner{}
	result := &returnedLifecycleAggregate{worker: worker}
	go worker.run() // want "goroutine is not joined on every return path"
	result.worker = &lifecycleOwner{}
	return result
}

type storedWorker struct {
	wait func()
}

func mutableCompletionTransferred(target *storedWorker) {
	var done chan struct{}
	wait := func() {
		if done != nil {
			<-done
		}
	}
	done = make(chan struct{})
	go func() { close(done) }()
	target.wait = wait
}

func nestedMutableCompletionReturned() (result storedWorker) {
	var done chan struct{}
	wait := func() {
		if done != nil {
			<-done
		}
	}
	done = make(chan struct{})
	go func() { close(done) }()
	result.wait = func() { wait() }
	return result
}

func deferredNestedMutableCompletion() (result storedWorker) {
	var done chan struct{}
	wait := func() {
		if done != nil {
			<-done
		}
	}
	defer func() {
		if result.wait == nil {
			wait()
		}
	}()
	done = make(chan struct{})
	go func() { close(done) }()
	result.wait = func() { wait() }
	return result
}
