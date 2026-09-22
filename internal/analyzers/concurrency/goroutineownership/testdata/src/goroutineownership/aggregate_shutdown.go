package goroutineownership

// Passing a channel's mutable aggregate owner to a shutdown helper can expose
// a completion path without proving which field the helper settles.
type shutdownChannels struct {
	commands chan bool
	done     chan struct{}
}

func signalShutdown(commands <-chan bool, done chan<- struct{}) {
	defer close(done)
	<-commands
}

func (channels *shutdownChannels) requestStop() {
	select {
	case channels.commands <- true:
	case <-channels.done:
	}
}

func helperSettlesStoredSignal() {
	channels := &shutdownChannels{}
	channels.commands = make(chan bool)
	channels.done = make(chan struct{})
	go signalShutdown(channels.commands, channels.done)
	defer func() { channels.requestStop() }()
}

func aggregateInspectionDoesNotSettle() {
	channels := &shutdownChannels{}
	channels.commands = make(chan bool)
	channels.done = make(chan struct{})
	go signalShutdown(channels.commands, channels.done) // want "goroutine is not joined on every return path"
	defer func() { _ = len(channels.done) }()
}

func differentAggregateDoesNotSettle() {
	channels := &shutdownChannels{}
	channels.commands = make(chan bool)
	channels.done = make(chan struct{})
	other := &shutdownChannels{commands: make(chan bool), done: make(chan struct{})}
	go signalShutdown(channels.commands, channels.done) // want "goroutine is not joined on every return path"
	defer func() { other.requestStop() }()
}
