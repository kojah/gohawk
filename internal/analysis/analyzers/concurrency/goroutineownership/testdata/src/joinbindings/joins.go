package joinbindings

import "sync"

func receive(done chan bool) { <-done }

func exactHelper() {
	done := make(chan bool)
	go func() { done <- true }()
	receive(done)
}

func mixedHelper(choice bool) {
	done := make(chan bool)
	go func() { done <- true }()
	selected := done
	if choice {
		selected = make(chan bool)
	}
	receive(selected)
}

type owner struct{ done chan bool }

func receiveField(value *owner) { <-value.done }

func replacedHelperField() {
	done := make(chan bool)
	go func() { done <- true }()
	value := &owner{done: done}
	value.done = make(chan bool)
	receiveField(value)
}

func exactWait() {
	var worker sync.WaitGroup
	worker.Add(1)
	go func() { defer worker.Done() }()
	worker.Wait()
}

func mixedWait(choice bool) {
	var worker, other sync.WaitGroup
	worker.Add(1)
	go func() { defer worker.Done() }()
	selected := &worker
	if choice {
		selected = &other
	}
	selected.Wait()
}

func unrelatedHelper() {
	done := make(chan bool)
	go func() { done <- true }() // want "goroutine is not joined on every return path"
	other := make(chan bool)
	close(other)
	receive(other)
}

type closer struct{ value bool }

func (value *closer) Close() {}

func mixedOwner(choice bool) {
	done := make(chan bool)
	value := &closer{value: true}
	go func() { done <- value.value }()
	selected := value
	if choice {
		selected = &closer{}
	}
	selected.Close()
}
