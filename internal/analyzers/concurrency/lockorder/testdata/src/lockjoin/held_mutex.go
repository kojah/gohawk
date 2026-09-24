package lockjoin

import "sync"

// The mutex need not be local. The root releases it only after the wait, so
// an outside release during the wait would make that later Unlock fatal on the
// path where the wait returns. The channel must still be fresh: an outside
// sender could satisfy the wait.
type heldOwner struct {
	mu    sync.Mutex
	rw    sync.RWMutex
	ptr   *sync.Mutex
	done  chan struct{}
	count int
	name  string
	run   func()
}

func (o *heldOwner) lockThenClose(done chan<- struct{}) {
	o.mu.Lock()
	close(done)
	o.mu.Unlock()
}

func (o *heldOwner) receiverMutex() {
	done := make(chan struct{})
	o.mu.Lock()
	go o.lockThenClose(done)
	<-done // want "waits for a worker that needs the held lock"
	o.mu.Unlock()
}

var packageMutex sync.Mutex

func packageMutexWorker() {
	done := make(chan struct{})
	packageMutex.Lock()
	go worker(&packageMutex, done)
	<-done // want "waits for a worker that needs the held lock"
	packageMutex.Unlock()
}

// Two parameters may name one mutex, but equality is not proved either way.
func possiblyAliasedOwners(parent, child *heldOwner) {
	done := make(chan struct{})
	parent.mu.Lock()
	go child.lockThenClose(done)
	<-done
	parent.mu.Unlock()
}

// A pointer loaded from a field can change; it is not an exact identity.
func loadedMutexPointer(o *heldOwner) {
	done := make(chan struct{})
	o.ptr.Lock()
	go worker(o.ptr, done)
	<-done
	o.ptr.Unlock()
}

// A receiver-owned channel may have senders outside this function.
func (o *heldOwner) receiverChannel() {
	o.mu.Lock()
	go o.lockThenClose(o.done)
	<-o.done
	o.mu.Unlock()
}

// Readers do not exclude each other, whoever owns the lock.
func (o *heldOwner) receiverReaders() {
	done := make(chan struct{})
	o.rw.RLock()
	go rwReadWorker(&o.rw, done)
	<-done
	o.rw.RUnlock()
}

// Reading, writing, and nil-checking unrelated receiver fields neither
// releases the mutex nor signals.
func (o *heldOwner) receiverReads() (string, int) {
	done := make(chan struct{})
	o.mu.Lock()
	o.count++
	count := o.count
	name := o.name
	if o.run == nil {
		o.name = "idle"
	}
	go o.lockThenClose(done)
	<-done // want "waits for a worker that needs the held lock"
	o.mu.Unlock()
	return name, count
}

// A function read from the receiver is opaque once called.
func (o *heldOwner) receiverCallback() {
	done := make(chan struct{})
	o.mu.Lock()
	go o.lockThenClose(done)
	o.run()
	<-done
	o.mu.Unlock()
}

// A mutex on an object read from the receiver may be replaced concurrently.
type heldChain struct{ next *heldOwner }

func (c *heldChain) loadedOwnerMutex() {
	done := make(chan struct{})
	c.next.mu.Lock()
	go c.next.lockThenClose(done)
	<-done
	c.next.mu.Unlock()
}
