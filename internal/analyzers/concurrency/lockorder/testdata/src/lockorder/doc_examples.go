package lockorder

import "sync"

//gohawk:example flagged
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

//gohawk:example end

//gohawk:example ok
var third sync.Mutex
var fourth sync.Mutex

func forwardSafely() {
	third.Lock()
	defer third.Unlock()
	fourth.Lock()
	defer fourth.Unlock()
}

func reverseSafely() {
	third.Lock()
	defer third.Unlock()
	fourth.Lock()
	defer fourth.Unlock()
}

//gohawk:example end
