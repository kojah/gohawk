package lockjoinhelper

import "sync"

// Send signals after obtaining its caller's mutex.
func Send(mu *sync.Mutex, ch chan<- int) {
	mu.Lock()
	mu.Unlock()
	ch <- 1
}

func Finish(mu *sync.Mutex, done chan<- struct{}) {
	mu.Lock()
	mu.Unlock()
	close(done)
}

func Launch(mu *sync.Mutex, done chan<- struct{}) {
	go Finish(mu, done)
}
