package serviceloops

import (
	"context"
	"sync"
)

// The gocronx-team/cron shape: after Stop, run has returned and nothing
// receives from add, so Schedule blocks forever.
type scheduler struct {
	add  chan int
	stop chan struct{}
}

func newScheduler() *scheduler {
	s := &scheduler{add: make(chan int), stop: make(chan struct{})}
	go s.run()
	return s
}

func (s *scheduler) run() {
	for {
		select {
		case <-s.add:
		case <-s.stop:
			return
		}
	}
}

func (s *scheduler) Schedule(v int) {
	s.add <- v // want "send can block forever after the service loop receiving it returns"
}

func (s *scheduler) Stop() { s.stop <- struct{}{} }

// The ErnestK/MCPSprut shape with an unbuffered channel: the loop returns when
// its context is cancelled.
type batcher struct{ updates chan string }

func newBatcher(ctx context.Context) *batcher {
	b := &batcher{updates: make(chan string)}
	go b.loop(ctx)
	return b
}

func (b *batcher) loop(ctx context.Context) {
	for {
		select {
		case <-b.updates:
		case <-ctx.Done():
			return
		}
	}
}

func (b *batcher) Submit(update string) {
	b.updates <- update // want "send can block forever after the service loop receiving it returns"
}

// Names carry no meaning: here the data channel is called stop and the
// shutdown channel is called add.
type misleadingNames struct {
	stop chan int
	add  chan struct{}
}

func newMisleadingNames() *misleadingNames {
	m := &misleadingNames{stop: make(chan int), add: make(chan struct{})}
	go m.run()
	return m
}

func (m *misleadingNames) run() {
	for {
		select {
		case <-m.stop:
		case <-m.add:
			return
		}
	}
}

func (m *misleadingNames) Send(v int) {
	m.stop <- v // want "send can block forever after the service loop receiving it returns"
}

func (m *misleadingNames) Close() { close(m.add) }

// A branch on an unrelated argument is not a lifecycle guard.
func (s *scheduler) ScheduleIfPositive(v int) {
	if v <= 0 {
		return
	}
	s.add <- v // want "send can block forever after the service loop receiving it returns"
}

// The gocronx-team/cron shape: the running flag is read and written with no
// lock, so Schedule can pass the check just as Stop ends the loop.
type unlockedGuard struct {
	running bool
	add     chan int
	stop    chan struct{}
}

func (u *unlockedGuard) Start() {
	u.running = true
	go u.run()
}

func (u *unlockedGuard) run() {
	for {
		select {
		case <-u.add:
		case <-u.stop:
			return
		}
	}
}

func (u *unlockedGuard) Schedule(v int) {
	if !u.running {
		return
	}
	u.add <- v // want "send can block forever after the service loop receiving it returns"
}

func (u *unlockedGuard) Stop() {
	if !u.running {
		return
	}
	u.stop <- struct{}{}
	u.running = false
}

func newUnlockedGuard() *unlockedGuard {
	return &unlockedGuard{add: make(chan int), stop: make(chan struct{})}
}

// A locked flag does not help when the loop can also stop on its context:
// cancellation needs no lock, so the flag can still say running.
type contextStoppedGuard struct {
	mu      sync.Mutex
	running bool
	add     chan int
}

func newContextStoppedGuard(ctx context.Context) *contextStoppedGuard {
	c := &contextStoppedGuard{add: make(chan int), running: true}
	go c.run(ctx)
	return c
}

func (c *contextStoppedGuard) run(ctx context.Context) {
	for {
		select {
		case <-c.add:
		case <-ctx.Done():
			return
		}
	}
}

func (c *contextStoppedGuard) Add(v int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return
	}
	c.add <- v // want "send can block forever after the service loop receiving it returns"
}

// The read is locked, but one writer clears the flag without the lock.
type unlockedWriter struct {
	mu      sync.Mutex
	running bool
	add     chan int
	stop    chan struct{}
}

func newUnlockedWriter() *unlockedWriter {
	u := &unlockedWriter{add: make(chan int), stop: make(chan struct{}), running: true}
	go u.run()
	return u
}

func (u *unlockedWriter) run() {
	for {
		select {
		case <-u.add:
		case <-u.stop:
			return
		}
	}
}

func (u *unlockedWriter) Add(v int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.running {
		return
	}
	u.add <- v // want "send can block forever after the service loop receiving it returns"
}

func (u *unlockedWriter) Stop() {
	u.mu.Lock()
	close(u.stop)
	u.mu.Unlock()
	u.running = false
}
