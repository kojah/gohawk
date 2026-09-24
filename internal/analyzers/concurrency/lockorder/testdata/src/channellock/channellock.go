package channellock

import (
	"sync"

	"lockjoinhelper"
)

func sendAfterLock(mu *sync.Mutex, ch chan<- int) {
	mu.Lock()
	mu.Unlock()
	ch <- 1
}

func blockedLocal() {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go sendAfterLock(&mu, ch)
	<-ch // want "receives while holding the lock needed by its sender"
	mu.Unlock()
}

func blockedImported() {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go lockjoinhelper.Send(&mu, ch)
	<-ch // want "receives while holding the lock needed by its sender"
	mu.Unlock()
}

// The send is the first signal; a later close does not make this a join-close
// report as well.
func sendThenClose(mu *sync.Mutex, ch chan<- int) {
	mu.Lock()
	mu.Unlock()
	ch <- 1
	close(ch)
}

func blockedSendThenClose() {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go sendThenClose(&mu, ch)
	<-ch // want "receives while holding the lock needed by its sender"
	mu.Unlock()
}

func sendBeforeLock(mu *sync.Mutex, ch chan<- int) {
	ch <- 1
	mu.Lock()
	mu.Unlock()
}

func signalFirst() {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go sendBeforeLock(&mu, ch)
	<-ch
	mu.Unlock()
}

func releaseBeforeReceive() {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go sendAfterLock(&mu, ch)
	mu.Unlock()
	<-ch
}

func otherMutex() {
	var parent, child sync.Mutex
	ch := make(chan int)
	parent.Lock()
	go sendAfterLock(&child, ch)
	<-ch
	parent.Unlock()
}

// A caller-owned lock is still held until this root's own Unlock.
func callerOwned(mu *sync.Mutex) {
	ch := make(chan int)
	mu.Lock()
	go sendAfterLock(mu, ch)
	<-ch // want "receives while holding the lock needed by its sender"
	mu.Unlock()
}

// A caller-owned channel may have a sender outside this function.
func callerChannel(ch chan int) {
	var mu sync.Mutex
	mu.Lock()
	go sendAfterLock(&mu, ch)
	<-ch
	mu.Unlock()
}

// A second participant might send before the blocked worker.
func anotherSender() {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go sendAfterLock(&mu, ch)
	go func() { ch <- 1 }()
	<-ch
	mu.Unlock()
}

// An unrelated child cannot satisfy the receive or release the held lock.
func blockedWithUnrelatedChild() {
	var mu sync.Mutex
	ch := make(chan int)
	other := make(chan struct{})
	mu.Lock()
	go sendAfterLock(&mu, ch)
	go func() { close(other) }()
	<-ch // want "receives while holding the lock needed by its sender"
	mu.Unlock()
}

// Both possible senders depend on the mutex held by the receiver.
func blockedWithTwoSenders() {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go sendAfterLock(&mu, ch)
	go sendAfterLock(&mu, ch)
	<-ch // want "receives while holding the lock needed by its sender"
	mu.Unlock()
}

// This worker can release the mutex before the sender acquires it.
func alternateUnlocker() {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go sendAfterLock(&mu, ch)
	go func() { mu.Unlock() }()
	<-ch
	mu.Unlock()
}

func maybeLock(mu *sync.Mutex, ch chan<- int, enabled bool) {
	if enabled {
		mu.Lock()
		mu.Unlock()
	}
	ch <- 1
}

func conditionalWorker(enabled bool) {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go maybeLock(&mu, ch, enabled)
	<-ch
	mu.Unlock()
}
