package goroutineownership

import "sync"

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

type resultEvent struct {
	Channel chan bool
}

// A channel read from a type-asserted event is owned by the event's producer,
// so a worker that answers on it transfers completion to that owner.
func answersOnAssertedEventChannel(event any) {
	var channel chan bool
	if typed, ok := event.(*resultEvent); ok {
		channel = typed.Channel
	}
	if channel != nil {
		go func(reply chan bool) { reply <- true }(channel)
	}
}

// A worker that closes an element of a captured slice signals through that
// slice. Returning the slice transfers every element's completion to the
// caller, so a readiness-only Done before the work is not a broken join.
func snapshotShardsIntoReturnedChannels(shards []map[string]int) []chan int {
	channels := make([]chan int, len(shards))
	var ready sync.WaitGroup
	ready.Add(len(shards))
	for index, shard := range shards {
		go func(index int, shard map[string]int) {
			channels[index] = make(chan int, len(shard))
			ready.Done()
			for _, value := range shard {
				channels[index] <- value
			}
			close(channels[index])
		}(index, shard)
	}
	ready.Wait()
	return channels
}

func snapshotShardsIntoLocalChannels(shards []map[string]int) {
	channels := make([]chan int, len(shards))
	var ready sync.WaitGroup
	ready.Add(len(shards))
	for index, shard := range shards {
		go func(index int, shard map[string]int) { // want "goroutine is not joined on every return path"
			channels[index] = make(chan int, len(shard))
			ready.Done()
			close(channels[index])
		}(index, shard)
	}
	ready.Wait()
}

type pendingWork struct {
	done chan struct{}
}

// A select that offers the pending work on a queue hands its completion to
// the receiver exactly as a plain send does.
func handedOffThroughSelectSend(queue chan<- *pendingWork, stop <-chan struct{}) {
	work := &pendingWork{done: make(chan struct{})}
	go func() { close(work.done) }()
	select {
	case queue <- work:
	case <-stop:
		<-work.done
	}
}

type subscriberHub struct {
	channels map[string][]chan string
}

// Channels ranged from the receiver's map are receiver-owned, so goroutines
// that close them transfer the obligation across the call boundary.
func (hub *subscriberHub) closeAll() {
	for _, chans := range hub.channels {
		for _, ch := range chans {
			go func(ch chan string) {
				ch <- "closing"
				close(ch)
			}(ch)
		}
	}
}

type pooledSession struct {
	done chan struct{}
}

func (session *pooledSession) run() {
	defer close(session.done)
}

// A session's done channel reached through an element of a local slice is
// shared with the registry that also holds the session, so launching the
// session is not this function's obligation to join.
func launchSessionsFromSlice(registry map[string]*pooledSession, names []string) {
	var starting []*pooledSession
	for _, name := range names {
		session := &pooledSession{done: make(chan struct{})}
		registry[name] = session
		starting = append(starting, session)
	}
	for _, session := range starting {
		go session.run()
	}
}

// replyItem holds the channel its worker answers on.
type replyItem struct{ replyCh chan int }

func answer(ch chan int) { ch <- 1 }

// bufferedReplyThroughField gives each worker a one-slot channel held in a
// struct field and receives the replies in a later loop that may stop early.
// The composite literal stores through one field address while the spawn reads
// the same field through another, so the buffer is only visible once stores are
// matched across sibling field addresses. The worker's single send cannot block,
// so an unreceived reply is not a leak. Nomad fetches server stats this way:
// https://github.com/hashicorp/nomad/blob/482b49bf1aec006f089bcfc7e632d8f6ac303e5e/nomad/stats_fetcher.go#L105-L126
func bufferedReplyThroughField(stop <-chan struct{}, count int) {
	var work []*replyItem
	for index := 0; index < count; index++ {
		item := &replyItem{replyCh: make(chan int, 1)}
		work = append(work, item)
		go answer(item.replyCh)
	}
	for _, item := range work {
		select {
		case <-item.replyCh:
		case <-stop:
		}
	}
}

// unbufferedReplyThroughField is the same shape without the buffer, so a worker
// whose reply is never received blocks forever.
func unbufferedReplyThroughField(stop <-chan struct{}, count int) {
	var work []*replyItem
	for index := 0; index < count; index++ {
		item := &replyItem{replyCh: make(chan int)}
		work = append(work, item)
		go answer(item.replyCh) // want "goroutine is not joined on every return path"
	}
	for _, item := range work {
		select {
		case <-item.replyCh:
		case <-stop:
		}
	}
}
