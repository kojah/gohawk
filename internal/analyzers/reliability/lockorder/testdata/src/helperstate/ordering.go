package helperstate

import (
	"mutexstate"
	"sync"
)

var first, second sync.Mutex

func forward() {
	mutexstate.Acquire(&first)
	second.Lock()
	second.Unlock()
	first.Unlock()
}

func backward() {
	mutexstate.Acquire(&second)
	first.Lock() // want "contradictory lock order"
	first.Unlock()
	second.Unlock()
}
