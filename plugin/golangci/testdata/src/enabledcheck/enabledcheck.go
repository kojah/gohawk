package enabledcheck

import "sync"

var first, second sync.Mutex

func forward() {
	first.Lock()
	defer first.Unlock()
	second.Lock()
	defer second.Unlock()
}

func reverse() {
	second.Lock()
	defer second.Unlock()
	first.Lock() // want "contradictory lock order"
	defer first.Unlock()
}
