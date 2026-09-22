package orderedhelpers

import (
	"mutexeffects"
	"sync"
)

var gate, guardedA, guardedB sync.Mutex

func guardedForward() {
	gate.Lock()
	defer gate.Unlock()
	mutexeffects.Pair(&guardedA, &guardedB)
}
func guardedBackward() {
	gate.Lock()
	defer gate.Unlock()
	mutexeffects.Pair(&guardedB, &guardedA)
}

var outer, directA, directB sync.Mutex

func directForward() {
	outer.Lock()
	defer outer.Unlock()
	directA.Lock()
	defer directA.Unlock()
	directB.Lock()
	directB.Unlock()
}
func directBackward() {
	outer.Lock()
	defer outer.Unlock()
	directB.Lock()
	defer directB.Unlock()
	directA.Lock()
	directA.Unlock()
}

var firstGate, secondGate, unguardedA, unguardedB sync.Mutex

func distinctGuardForward() {
	firstGate.Lock()
	defer firstGate.Unlock()
	mutexeffects.Pair(&unguardedA, &unguardedB)
}
func distinctGuardBackward() {
	secondGate.Lock()
	defer secondGate.Unlock()
	mutexeffects.Pair(&unguardedB, &unguardedA) // want "contradictory lock order"
}

var readGate sync.RWMutex
var readA, readB sync.Mutex

func readGuardForward() {
	readGate.RLock()
	defer readGate.RUnlock()
	mutexeffects.Pair(&readA, &readB)
}
func readGuardBackward() {
	readGate.RLock()
	defer readGate.RUnlock()
	mutexeffects.Pair(&readB, &readA) // want "contradictory lock order"
}

var releasedGate, releasedA, releasedB sync.Mutex

func releasedGuardForward() {
	releasedGate.Lock()
	releasedGate.Unlock()
	mutexeffects.Pair(&releasedA, &releasedB)
}
func releasedGuardBackward() {
	releasedGate.Lock()
	releasedGate.Unlock()
	mutexeffects.Pair(&releasedB, &releasedA) // want "contradictory lock order"
}
