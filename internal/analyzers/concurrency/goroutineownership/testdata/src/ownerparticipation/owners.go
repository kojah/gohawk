package ownerparticipation

type owner struct{ value bool }

// These deliberately have familiar lifecycle names without joining a worker.
func (value *owner) Close()    {}
func (value *owner) Stop()     {}
func (value *owner) Shutdown() {}
func (value *owner) Wait()     {}
func (value *owner) Kill()     {}

func directClose() {
	done := make(chan bool)
	value := &owner{value: true}
	go func() { done <- value.value }()
	value.Close()
}

func deferredClose() {
	done := make(chan bool)
	value := &owner{value: true}
	go func() { done <- value.value }()
	defer value.Close()
}

func closeOwner(value *owner) { value.Close() }

func helperClose() {
	done := make(chan bool)
	value := &owner{value: true}
	go func() { done <- value.value }()
	closeOwner(value)
}

func directStop() {
	done := make(chan bool)
	value := &owner{value: true}
	go func() { done <- value.value }()
	value.Stop()
}

func directShutdown() {
	done := make(chan bool)
	value := &owner{value: true}
	go func() { done <- value.value }()
	value.Shutdown()
}

func directWait() {
	done := make(chan bool)
	value := &owner{value: true}
	go func() { done <- value.value }()
	value.Wait()
}

func directKill() {
	done := make(chan bool)
	value := &owner{value: true}
	go func() { done <- value.value }()
	value.Kill()
}

func closeAndJoin() {
	done := make(chan bool)
	value := &owner{value: true}
	go func() { done <- value.value }()
	value.Close()
	<-done
}

func closeOther() {
	done := make(chan bool)
	value := &owner{value: true}
	go func() { done <- value.value }() // want "goroutine is not joined on every return path"
	other := &owner{}
	other.Close()
}
