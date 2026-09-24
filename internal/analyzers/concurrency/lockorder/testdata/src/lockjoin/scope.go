package lockjoin

import "sync"

// Operations on a disjoint resource do not supply a partner for done or
// release mu. Every operation and participant still needs a complete model.
func disjointEffects() {
	var mu, other sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	other.Lock()
	other.Unlock()
	go worker(&mu, done)
	<-done // want "waits for a worker that needs the held lock"
	mu.Unlock()
}

func aliasEffects() {
	var mu sync.Mutex
	p := &mu
	done := make(chan struct{})
	mu.Lock()
	p.Unlock()
	go worker(&mu, done)
	<-done
}

func opaqueParticipant(f func(*sync.Mutex)) {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go worker(&mu, done)
	f(&mu)
	<-done
	mu.Unlock()
}

type owner struct{ mu sync.Mutex }

func (o *owner) run(done chan<- struct{})   { worker(&o.mu, done) }
func (o *owner) start(done chan<- struct{}) { go o.run(done) }

func receiverWorker() {
	var o owner
	done := make(chan struct{})
	o.mu.Lock()
	o.start(done)
	<-done // want "waits for a worker that needs the held lock"
	o.mu.Unlock()
}

func separateReceivers() {
	var parent, child owner
	done := make(chan struct{})
	parent.mu.Lock()
	child.start(done)
	<-done
	parent.mu.Unlock()
}

func externalReceiver(o *owner) {
	done := make(chan struct{})
	o.mu.Lock()
	o.start(done)
	<-done
	o.mu.Unlock()
}

func (o *owner) stop(done <-chan struct{}) {
	<-done // want "waits for a worker that needs the held lock"
	o.mu.Unlock()
}

func startThenStop() {
	var o owner
	done := make(chan struct{})
	o.mu.Lock()
	o.start(done)
	o.stop(done)
}
