package oncepolicy

import "sync"

func initialize() {}

//gohawk:example flagged
func start() {
	sync.OnceFunc(initialize)() // want "sync.OnceFunc wrapper is discarded after one call"
}

//gohawk:example end

var initializeOnce = sync.OnceFunc(initialize)

//gohawk:example ok
func startOnce() {
	initializeOnce()
}

//gohawk:example end
