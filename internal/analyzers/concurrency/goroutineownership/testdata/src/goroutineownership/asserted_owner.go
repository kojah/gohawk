package goroutineownership

// A checked interface assertion changes the owner's static type, not the
// completion channel subsequently stored into it. Only a later handoff owns
// that channel; merely constructing and filling the owner is not enough.
func newSignalContainer() any { return &producedSignal{} }

func assertedOwnerHandoff(run func(*producedSignal)) {
	owner := newSignalContainer().(*producedSignal)
	done := make(chan struct{})
	owner.done = done
	run(owner)
	go func() { close(done) }()
}

func assertedOwnerNotHandedOff() {
	owner := newSignalContainer().(*producedSignal)
	done := make(chan struct{})
	owner.done = done
	go func() { close(done) }() // want "goroutine is not joined on every return path"
}

func assertedOwnerCarriesDifferentSignal(run func(*producedSignal)) {
	owner := newSignalContainer().(*producedSignal)
	done := make(chan struct{})
	owner.done = make(chan struct{})
	run(owner)
	go func() { close(done) }() // want "goroutine is not joined on every return path"
}

func assertedOwnerOnlyRead() {
	owner := newSignalContainer().(*producedSignal)
	done := make(chan struct{})
	owner.done = done
	inspectSignalOwner(owner)
	go func() { close(done) }() // want "goroutine is not joined on every return path"
}

func inspectSignalOwner(owner *producedSignal) { _ = len(owner.done) }
