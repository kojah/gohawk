package branchdep

import "sync"

type Failure struct{}

func (*Failure) Error() string { return "failure" }

// Pick, Mode, and Acquire have different effects on their branches, so their
// facts publish path alternatives.
func Pick(a, b chan int, flag bool) {
	if flag {
		close(a)
	} else {
		close(b)
	}
}

func Mode(a, b chan int, n int) {
	if n == 1 {
		close(a)
	} else {
		close(b)
	}
}

func Acquire(mu *sync.Mutex, n int) error {
	if n < 0 {
		return &Failure{}
	}
	mu.Lock()
	return nil
}
