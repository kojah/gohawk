package serviceloops

import (
	"context"
	"sync"
	"sync/atomic"
)

// Each type below is one way a send to a service loop can look like the
// reported shape and still be safe, or at least not provably stuck.

// The send selects on the loop's stop signal, so it has a way out.
type selectingSender struct {
	add  chan int
	stop chan struct{}
}

func newSelectingSender() *selectingSender {
	s := &selectingSender{add: make(chan int), stop: make(chan struct{})}
	go s.run()
	return s
}

func (s *selectingSender) run() {
	for {
		select {
		case <-s.add:
		case <-s.stop:
			return
		}
	}
}

func (s *selectingSender) Add(v int) {
	select {
	case s.add <- v:
	case <-s.stop:
	}
}

// A running flag checked under a lock is the robfig/cron guard: the flag is
// read and written, and the loop is stopped, only while holding the same
// mutex, so no caller can reach the send after the loop stops.
type guardedSender struct {
	mu      sync.Mutex
	running bool
	add     chan int
	stop    chan struct{}
}

func newGuardedSender() *guardedSender {
	g := &guardedSender{add: make(chan int), stop: make(chan struct{}), running: true}
	go g.run()
	return g
}

func (g *guardedSender) run() {
	for {
		select {
		case <-g.add:
		case <-g.stop:
			return
		}
	}
}

func (g *guardedSender) Add(v int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.running {
		return
	}
	g.add <- v
}

// A loop with no way to return keeps receiving for the life of the process.
type foreverLoop struct{ add chan int }

func newForeverLoop() *foreverLoop {
	f := &foreverLoop{add: make(chan int)}
	go f.run()
	return f
}

func (f *foreverLoop) run() {
	for {
		<-f.add
	}
}

func (f *foreverLoop) Add(v int) { f.add <- v }

// A loop that ranges until the channel is closed: after close, a send panics
// rather than blocks, which is a different defect.
type closedLoop struct{ add chan int }

func newClosedLoop() *closedLoop {
	c := &closedLoop{add: make(chan int)}
	go c.run()
	return c
}

func (c *closedLoop) run() {
	for range c.add {
	}
}

func (c *closedLoop) Add(v int) { c.add <- v }
func (c *closedLoop) Close()    { close(c.add) }

// A buffered channel absorbs sends after the loop stops; how many is a
// cardinality this check does not guess.
type bufferedLoop struct {
	add  chan int
	stop chan struct{}
}

func newBufferedLoop() *bufferedLoop {
	b := &bufferedLoop{add: make(chan int, 16), stop: make(chan struct{})}
	go b.run()
	return b
}

func (b *bufferedLoop) run() {
	for {
		select {
		case <-b.add:
		case <-b.stop:
			return
		}
	}
}

func (b *bufferedLoop) Add(v int) { b.add <- v }

// A channel supplied by the caller may have receivers this package cannot see.
type suppliedChannel struct {
	add  chan int
	stop chan struct{}
}

func newSuppliedChannel(add chan int) *suppliedChannel {
	s := &suppliedChannel{add: add, stop: make(chan struct{})}
	go s.run()
	return s
}

func (s *suppliedChannel) run() {
	for {
		select {
		case <-s.add:
		case <-s.stop:
			return
		}
	}
}

func (s *suppliedChannel) Add(v int) { s.add <- v }

// Handing the channel out lets other code receive from it.
type exposedChannel struct {
	add  chan int
	stop chan struct{}
}

func newExposedChannel() *exposedChannel {
	e := &exposedChannel{add: make(chan int), stop: make(chan struct{})}
	go e.run()
	return e
}

func (e *exposedChannel) run() {
	for {
		select {
		case <-e.add:
		case <-e.stop:
			return
		}
	}
}

func (e *exposedChannel) Updates() chan int { return e.add }
func (e *exposedChannel) Add(v int)         { e.add <- v }

// An exported field can be received from in another package.
type ExportedChannel struct {
	Add  chan int
	stop chan struct{}
}

func NewExportedChannel() *ExportedChannel {
	e := &ExportedChannel{Add: make(chan int), stop: make(chan struct{})}
	go e.run()
	return e
}

func (e *ExportedChannel) run() {
	for {
		select {
		case <-e.Add:
		case <-e.stop:
			return
		}
	}
}

func (e *ExportedChannel) Send(v int) { e.Add <- v }

// A receiver that also runs synchronously is not only a background loop.
type synchronousReceiver struct {
	add  chan int
	stop chan struct{}
}

func newSynchronousReceiver() *synchronousReceiver {
	s := &synchronousReceiver{add: make(chan int), stop: make(chan struct{})}
	go s.run()
	return s
}

func (s *synchronousReceiver) run() {
	for {
		select {
		case <-s.add:
		case <-s.stop:
			return
		}
	}
}

func (s *synchronousReceiver) Drain()    { s.run() }
func (s *synchronousReceiver) Add(v int) { s.add <- v }

// A second receiver that never returns keeps the channel served.
type backupReceiver struct {
	add  chan int
	stop chan struct{}
}

func newBackupReceiver() *backupReceiver {
	b := &backupReceiver{add: make(chan int), stop: make(chan struct{})}
	go b.run()
	go b.drain()
	return b
}

func (b *backupReceiver) run() {
	for {
		select {
		case <-b.add:
		case <-b.stop:
			return
		}
	}
}

func (b *backupReceiver) drain() {
	for {
		<-b.add
	}
}

func (b *backupReceiver) Add(v int) { b.add <- v }

// A send made by the loop's own launcher happens while the loop runs.
type launcherSend struct {
	add  chan int
	stop chan struct{}
}

func (l *launcherSend) run() {
	for {
		select {
		case <-l.add:
		case <-l.stop:
			return
		}
	}
}

func (l *launcherSend) Start(v int) {
	l.add, l.stop = make(chan int), make(chan struct{})
	go l.run()
	l.add <- v
}

// A receiver that takes one value and returns is not a service loop.
type unconditionalReturn struct{ add chan int }

func newUnconditionalReturn() *unconditionalReturn {
	u := &unconditionalReturn{add: make(chan int)}
	go u.run()
	return u
}

func (u *unconditionalReturn) run() {
	<-u.add
}

func (u *unconditionalReturn) Add(v int) { u.add <- v }

// The loop has a stop arm, but nothing ever sends on or closes that channel,
// so the loop cannot actually return.
type unsignalledStop struct {
	add  chan int
	stop chan struct{}
}

func newUnsignalledStop() *unsignalledStop {
	u := &unsignalledStop{add: make(chan int), stop: make(chan struct{})}
	go u.run()
	return u
}

func (u *unsignalledStop) run() {
	for {
		select {
		case <-u.add:
		case <-u.stop:
			return
		}
	}
}

func (u *unsignalledStop) Add(v int) { u.add <- v }

// The loop stops when its context is cancelled, and the sender selects on the
// same context, so it has a way out.
type contextSender struct{ add chan int }

func newContextSender(ctx context.Context) *contextSender {
	c := &contextSender{add: make(chan int)}
	go c.run(ctx)
	return c
}

func (c *contextSender) run(ctx context.Context) {
	for {
		select {
		case <-c.add:
		case <-ctx.Done():
			return
		}
	}
}

func (c *contextSender) Add(ctx context.Context, v int) {
	select {
	case c.add <- v:
	case <-ctx.Done():
	}
}

func (s *selectingSender) Stop() { close(s.stop) }

func (g *guardedSender) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running {
		close(g.stop)
		g.running = false
	}
}

func (b *bufferedLoop) Stop() { close(b.stop) }

func (s *suppliedChannel) Stop() { close(s.stop) }

func (e *exposedChannel) Stop() { close(e.stop) }

func (e *ExportedChannel) Stop() { close(e.stop) }

func (s *synchronousReceiver) Stop() { close(s.stop) }

func (b *backupReceiver) Stop() { close(b.stop) }

func (l *launcherSend) Stop() { close(l.stop) }

// An unexported helper is reached through entry points this check does not
// follow; here the only caller checks the running flag first.
type helperSender struct {
	mu      sync.Mutex
	running bool
	add     chan int
	stop    chan struct{}
}

func newHelperSender() *helperSender {
	h := &helperSender{add: make(chan int), stop: make(chan struct{}), running: true}
	go h.run()
	return h
}

func (h *helperSender) run() {
	for {
		select {
		case <-h.add:
		case <-h.stop:
			return
		}
	}
}

func (h *helperSender) enqueue(v int) { h.add <- v }

func (h *helperSender) Add(v int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running {
		h.enqueue(v)
	}
}

func (h *helperSender) Stop() { close(h.stop) }

// A loop that sends on the channel it serves is a self-deadlock, a different
// defect from a stopped loop.
type selfSender struct {
	add  chan int
	stop chan struct{}
}

func newSelfSender() *selfSender {
	s := &selfSender{add: make(chan int), stop: make(chan struct{})}
	go s.run()
	return s
}

func (s *selfSender) run() {
	for {
		select {
		case v := <-s.add:
			if v > 0 {
				s.add <- v - 1
			}
		case <-s.stop:
			return
		}
	}
}

func (s *selfSender) Stop() { close(s.stop) }

// A guard behind a method call is not something this check can read.
type methodGuard struct {
	mu      sync.Mutex
	running bool
	add     chan int
	stop    chan struct{}
}

func newMethodGuard() *methodGuard {
	m := &methodGuard{add: make(chan int), stop: make(chan struct{}), running: true}
	go m.run()
	return m
}

func (m *methodGuard) run() {
	for {
		select {
		case <-m.add:
		case <-m.stop:
			return
		}
	}
}

func (m *methodGuard) isRunning() bool { return m.running }

func (m *methodGuard) Add(v int) {
	if !m.isRunning() {
		return
	}
	m.add <- v
}

func (m *methodGuard) Stop() { close(m.stop) }

// A flag written through an atomic hands its address to other code.
type atomicGuard struct {
	mu    sync.Mutex
	state int32
	add   chan int
	stop  chan struct{}
}

func newAtomicGuard() *atomicGuard {
	a := &atomicGuard{add: make(chan int), stop: make(chan struct{})}
	go a.run()
	return a
}

func (a *atomicGuard) run() {
	for {
		select {
		case <-a.add:
		case <-a.stop:
			return
		}
	}
}

func (a *atomicGuard) Add(v int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state == 0 {
		return
	}
	a.add <- v
}

func (a *atomicGuard) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	atomic.StoreInt32(&a.state, 0)
	close(a.stop)
}
