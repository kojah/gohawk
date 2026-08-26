package lockorder

import "sync"

var first sync.Mutex
var second sync.Mutex

func forward() {
	first.Lock()
	defer first.Unlock()
	second.Lock()
	defer second.Unlock()
}

func reverse() {
	second.Lock()
	defer second.Unlock()
	first.Lock() // want "contradictory lock order: first and second"
	defer first.Unlock()
}
