package goroutineownership

// Copying a factory's returned aggregate into a local does not establish a
// fresh, exclusively owned completion channel. Factory-returned owner copies
// remain unknown, even when callers do not consume their companion channel.
type producedSignal struct{ done chan struct{} }

func makeProducedSignal() (producedSignal, <-chan struct{}) {
	done := make(chan struct{})
	return producedSignal{done: done}, done
}

func (producer *producedSignal) run() { close(producer.done) }

func consumesFactoryCompanion() {
	producer, done := makeProducedSignal()
	go producer.run()
	for range done {
	}
}

func localProducerStillNeedsJoin() {
	producer := producedSignal{done: make(chan struct{})}
	go producer.run() // want "goroutine is not joined on every return path"
}
