package orderedhelpers

import (
	"mutexeffects"
	"sync"
)

var first, second sync.Mutex

func forward()  { mutexeffects.Pair(&first, &second) }
func backward() { mutexeffects.Pair(&second, &first) } // want "contradictory lock order"

var before, after sync.Mutex

func acquisitionBeforeRelease() {
	before.Lock()
	mutexeffects.AcquireThenRelease(&before, &after)
}
func opposite() { mutexeffects.Pair(&after, &before) } // want "contradictory lock order"
