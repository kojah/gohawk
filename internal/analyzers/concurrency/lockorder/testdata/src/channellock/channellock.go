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

// A caller-owned lock can be released by someone else.
func callerOwned(mu *sync.Mutex) {
	ch := make(chan int)
	mu.Lock()
	go sendAfterLock(mu, ch)
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
