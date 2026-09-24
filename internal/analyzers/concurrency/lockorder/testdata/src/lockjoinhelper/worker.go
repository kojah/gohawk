package lockjoinhelper

import "sync"

func Finish(mu *sync.Mutex, done chan<- struct{}) {
	mu.Lock()
	mu.Unlock()
	close(done)
}
