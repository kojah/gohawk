package mixedcycles

import "sync"

func lockAndJoin() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go func() { mu.Lock(); mu.Unlock(); close(done) }()
	<-done // want "waiting for a worker while holding the mutex"
	mu.Unlock()
}

func lockAndGroup() {
	var mu sync.Mutex
	var group sync.WaitGroup
	group.Add(1)
	mu.Lock()
	defer mu.Unlock()
	go func() { defer group.Done(); mu.Lock(); defer mu.Unlock() }()
	group.Wait() // want "waiting for a worker while holding the mutex"
}

func sendUnderLock() {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go func() { mu.Lock(); <-ch; mu.Unlock() }()
	ch <- 1 // want "channel operation holds the mutex"
	mu.Unlock()
}

func receiveUnderLock() {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go func() { mu.Lock(); ch <- 1; mu.Unlock() }()
	<-ch // want "channel operation holds the mutex"
	mu.Unlock()
}
