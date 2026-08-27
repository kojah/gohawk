package lockorder

import "sync"

var first sync.Mutex
var second sync.Mutex
var third sync.Mutex
var fourth sync.Mutex

//gohawk:example flagged
func forward() { first.Lock(); defer first.Unlock(); second.Lock(); defer second.Unlock() }
func reverse() { second.Lock(); defer second.Unlock(); first.Lock(); defer first.Unlock() } // want "contradictory lock order: first and second"
//gohawk:example end

//gohawk:example ok
func forwardSafely() { third.Lock(); defer third.Unlock(); fourth.Lock(); defer fourth.Unlock() }
func reverseSafely() { third.Lock(); defer third.Unlock(); fourth.Lock(); defer fourth.Unlock() }

//gohawk:example end
