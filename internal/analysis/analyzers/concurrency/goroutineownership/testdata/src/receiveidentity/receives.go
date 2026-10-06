package receiveidentity

import "sync"

// A receive or Wait selected from several handles supplies possible completion,
// not an exact join. Exact and unrelated handles pin the two other outcomes.

func directMixed(flag bool) {
	done := make(chan bool)
	other := make(chan bool)
	close(other)
	go func() { done <- true }()
	selected := done
	if flag {
		selected = other
	}
	<-selected
}

func helperMixedReceive(done <-chan bool, flag bool) {
	other := make(chan bool)
	close(other)
	selected := done
	if flag {
		selected = other
	}
	<-selected
}

func helperMixed(flag bool) {
	done := make(chan bool)
	go func() { done <- true }()
	helperMixedReceive(done, flag)
}

func helperMixedWait(group *sync.WaitGroup, flag bool) {
	other := new(sync.WaitGroup)
	selected := group
	if flag {
		selected = other
	}
	selected.Wait()
}

func groupMixed(flag bool) {
	group := new(sync.WaitGroup)
	group.Add(1)
	go func() { defer group.Done() }()
	helperMixedWait(group, flag)
}

func exact() {
	done := make(chan bool)
	go func() { done <- true }()
	<-done
}

func selectedMixed(flag bool) {
	done := make(chan bool)
	other := make(chan bool)
	close(other)
	go func() { done <- true }()
	selected := done
	if flag {
		selected = other
	}
	select {
	case <-selected:
	}
}

func receive(done <-chan bool) { <-done }

func receiveMixedForward(done <-chan bool, flag bool) {
	other := make(chan bool)
	close(other)
	selected := done
	if flag {
		selected = other
	}
	receive(selected)
}

func nestedMixed(flag bool) {
	done := make(chan bool)
	go func() { done <- true }()
	receiveMixedForward(done, flag)
}

func exactHelper() {
	done := make(chan bool)
	go func() { done <- true }()
	receive(done)
}

func unrelated() {
	done := make(chan bool)
	other := make(chan bool)
	close(other)
	go func() { done <- true }() // want "goroutine is not joined on every return path"
	<-other
}

func selectedEdge(flag bool) {
	done := make(chan bool)
	other := make(chan bool)
	close(other)
	go func() { done <- true }()
	selected := done
	if flag {
		selected = other
	}
	select {
	case <-selected:
		return
	case <-done:
		return
	}
}

func mixedDefault(flag bool) {
	done := make(chan bool)
	other := make(chan bool)
	close(other)
	go func() { done <- true }() // want "goroutine is not joined on every return path"
	selected := done
	if flag {
		selected = other
	}
	select {
	case <-selected:
	default:
		return
	}
}

type handles struct{ first, second chan bool }

func siblingField() {
	h := &handles{make(chan bool), make(chan bool)}
	close(h.second)
	go func() { h.first <- true }()
	<-h.second
}

func replacedReceive(done <-chan bool) {
	other := make(chan bool)
	close(other)
	selected := new(<-chan bool)
	*selected = done
	*selected = other
	<-*selected
}

func helperReplaced() {
	done := make(chan bool)
	go func() { done <- true }()
	replacedReceive(done)
}
