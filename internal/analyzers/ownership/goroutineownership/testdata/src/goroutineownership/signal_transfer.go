package goroutineownership

// This file covers direct signal observation and ownership transferred through
// fields, registries, maps, and caller-owned lifecycle aggregates.

func joinedByClose() {
	done := make(chan struct{})
	go func() { close(done) }()
	<-done
}

func joinedByDeferredSend() {
	done := make(chan any)
	// A deferred closure may publish the terminal result while preserving panic
	// handling. The matching receive is still concrete completion evidence:
	// https://github.com/uber-go/zap/blob/bb1a55dd13257cf7cbd06b4146674c67ca614dea/logger_test.go#L946-L956
	go func() {
		defer func() { done <- recover() }()
		panic("finished")
	}()
	<-done
}

func joinedThroughAlias() {
	done := make(chan struct{})
	alias := done
	go func() { close(alias) }()
	<-done
}

func conditionallyJoined(join bool) {
	done := make(chan struct{})
	go func() { close(done) }() // want "goroutine is not joined on every return path"
	if join {
		<-done
	}
}

type signalOwner struct {
	done <-chan struct{}
}

func transferredSignal(owner *signalOwner) {
	done := make(chan struct{})
	go func() { close(done) }()
	owner.done = done
}

type signalRegistry struct{}

func (*signalRegistry) Register(<-chan struct{}) {}

func registeredAfterSpawn(registry *signalRegistry) {
	done := make(chan struct{})
	go func() { close(done) }() // want "goroutine is not joined on every return path"
	registry.Register(done)
}

type opaqueSignalRegistry interface {
	Register(<-chan struct{})
	RegisterOwner(signalOwner)
}

func opaqueRegistrationBeforeSpawn(registry opaqueSignalRegistry) {
	done := make(chan struct{})
	registry.Register(done)
	go func() { close(done) }()
}

func wrapSignal(done <-chan struct{}) signalOwner {
	return signalOwner{done: done}
}

func opaqueWrappedRegistrationAfterSpawn(registry opaqueSignalRegistry) {
	done := make(chan struct{})
	go func() { close(done) }()
	registry.RegisterOwner(wrapSignal(done))
}

func opaqueWrappedRegistrationBeforeSpawn(registry opaqueSignalRegistry) {
	done := make(chan struct{})
	registry.RegisterOwner(wrapSignal(done))
	go func() { close(done) }()
}

func transferredSignalBeforeSpawn(owner *signalOwner) {
	done := make(chan struct{})
	owner.done = done
	go func() { close(done) }()
}

func transferredLifecycleMap(owners map[string]*lifecycleOwner) {
	owner := &lifecycleOwner{}
	go owner.run()
	owners["worker"] = owner
}

type nestedLifecycleOwner struct {
	worker *lifecycleOwner
}

func startsNestedCallerOwner(owner nestedLifecycleOwner) {
	go owner.worker.run() // want "goroutine is not joined on every return path"
}
