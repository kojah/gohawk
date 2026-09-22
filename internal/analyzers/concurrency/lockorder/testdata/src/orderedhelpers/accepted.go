package orderedhelpers

import (
	"mutexeffects"
	"sync"
)

var releaseFirst, releaseSecond sync.Mutex

// Release-before-acquire must not fabricate an overlapping held set.
func releaseBeforeAcquisition() {
	releaseFirst.Lock()
	mutexeffects.ReleaseThenAcquire(&releaseFirst, &releaseSecond)
}
func oppositeButDisjoint() {
	mutexeffects.Pair(&releaseSecond, &releaseFirst)
}

var conditionalFirst, conditionalSecond sync.Mutex

// An unavailable fact is not a complete empty summary or unconditional pair.
func conditionalOrder(yes bool) { mutexeffects.Conditional(&conditionalFirst, &conditionalSecond, yes) }
func conditionalOpposite()      { mutexeffects.Pair(&conditionalSecond, &conditionalFirst) }
