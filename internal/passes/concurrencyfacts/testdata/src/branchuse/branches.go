package branchuse

import (
	"branchdep"
	"sync"
)

func pickTwice(a, b chan int, x bool) {
	branchdep.Pick(a, b, x)
	branchdep.Pick(a, b, x)
}

func modeConstant(a, b chan int) { branchdep.Mode(a, b, 1) }

func useAcquire(mu *sync.Mutex, n int) {
	if branchdep.Acquire(mu, n) != nil {
		return
	}
	mu.Unlock()
}
