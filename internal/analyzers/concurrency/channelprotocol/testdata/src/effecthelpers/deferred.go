package effecthelpers

import "sync"

func Finish(mu *sync.Mutex, done chan struct{}) { mu.Unlock(); close(done) }
func DeferredWorker(mu *sync.Mutex, done chan struct{}) {
	mu.Lock()
	defer Finish(mu, done)
}

func PreludeWorker(mu, other *sync.Mutex, done chan struct{}) {
	other.Lock()
	other.Unlock()
	DeferredWorker(mu, done)
}
