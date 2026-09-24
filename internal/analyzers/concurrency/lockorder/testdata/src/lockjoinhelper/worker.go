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

type AcquireError struct{}

func (*AcquireError) Error() string { return "acquire failed" }

// Acquire locks only on success; its fact publishes both paths and what each
// returns.
func Acquire(mu *sync.Mutex, n int) error {
	if n < 0 {
		return &AcquireError{}
	}
	mu.Lock()
	return nil
}
