package mutexeffects

import "sync"

func Pair(first, second *sync.Mutex) {
	first.Lock()
	second.Lock()
	second.Unlock()
	first.Unlock()
}

func ReleaseThenAcquire(first, second *sync.Mutex) {
	first.Unlock()
	second.Lock()
	second.Unlock()
}

func AcquireThenRelease(first, second *sync.Mutex) {
	second.Lock()
	second.Unlock()
	first.Unlock()
}

func Conditional(first, second *sync.Mutex, yes bool) {
	if yes {
		first.Lock()
		second.Lock()
		second.Unlock()
		first.Unlock()
	}
}
