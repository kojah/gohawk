package lockjoin

import (
	"context"
	"sync"
)

// Every possible arm must preserve the dependency. Cancellation, a default
// arm, or an early signal must not disappear into an unconditional sequence.
func cancellationEscape() {
	var mu sync.Mutex
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	mu.Lock()
	go func() {
		select {
		case <-ctx.Done():
			close(done)
		default:
			worker(&mu, done)
		}
	}()
	cancel()
	<-done
	mu.Unlock()
}

// When flag is true the parent waits while holding the lock, which is one
// feasible execution; a deadlock need not happen on every path.
func optionalParentWait(flag bool) {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go worker(&mu, done)
	if flag {
		<-done // want "waits for a worker that needs the held lock"
	}
	mu.Unlock()
}

func branchWorker(mu *sync.Mutex, done chan<- struct{}, flag bool) {
	if flag {
		mu.Lock()
		close(done)
		mu.Unlock()
	} else {
		worker(mu, done)
	}
}

func launchBranchWorker(mu *sync.Mutex, done chan<- struct{}, flag bool) {
	go branchWorker(mu, done, flag)
}

func everyWorkerBranchBlocked(flag bool) {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	launchBranchWorker(&mu, done, flag)
	<-done // want "waits for a worker that needs the held lock"
	mu.Unlock()
}
