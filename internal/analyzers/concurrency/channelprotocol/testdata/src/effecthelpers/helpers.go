package effecthelpers

import "sync"

func Worker(mu *sync.Mutex, done chan struct{})  { mu.Lock(); mu.Unlock(); close(done) }
func Receive(mu *sync.Mutex, ch chan int)        { mu.Lock(); <-ch; mu.Unlock() }
func SendResult(ch chan int, done chan struct{}) { ch <- 1; close(done) }
func Wait(done chan struct{})                    { <-done }
func Unlock(mu *sync.Mutex)                      { mu.Unlock() }
func Early(done chan struct{}, mu *sync.Mutex)   { close(done); mu.Lock(); mu.Unlock() }
func Conditional(mu *sync.Mutex, done chan struct{}, skip bool) {
	if !skip {
		mu.Lock()
		mu.Unlock()
	}
	close(done)
}
func Reset(mu *sync.Mutex)      { *mu = sync.Mutex{} }
func Launch(done chan struct{}) { go func() { close(done) }() }
func Pure(n int) int            { return n + 1 }

// Identity and order must survive a second importing package.
func Forward(mu *sync.Mutex, done chan struct{}) { Worker(mu, done) }

func AddOne(group *sync.WaitGroup) { group.Add(1) }
func Join(group *sync.WaitGroup) { group.Wait() }
func GroupWorker(mu *sync.Mutex, group *sync.WaitGroup) {
	defer group.Done()
	mu.Lock()
	defer mu.Unlock()
}
