package lockjoin

import "sync"

func rwReadWorker(mu *sync.RWMutex, done chan<- struct{}) {
	mu.RLock()
	close(done)
	mu.RUnlock()
}

func concurrentReaders() {
	var mu sync.RWMutex
	done := make(chan struct{})
	mu.RLock()
	go rwReadWorker(&mu, done)
	<-done
	mu.RUnlock()
}

func writerBlocksReader() {
	var mu sync.RWMutex
	done := make(chan struct{})
	mu.Lock()
	go rwReadWorker(&mu, done)
	<-done // want "waits for a worker that needs the held lock"
	mu.Unlock()
}

func readerBlocksWriter() {
	var mu sync.RWMutex
	done := make(chan struct{})
	mu.RLock()
	go func() { mu.Lock(); close(done); mu.Unlock() }()
	<-done // want "waits for a worker that needs the held lock"
	mu.RUnlock()
}
