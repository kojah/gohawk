package callbackuse

import (
	"callbackdep"
	"sync"
)

func closeUnderLock(mu *sync.Mutex, done chan int) {
	callbackdep.WithLock(mu, func() { close(done) })
}

func forwardHole(mu *sync.Mutex, f func()) {
	callbackdep.WithLock(mu, f)
}
