package oncepolicy

import "sync"

func regressionInitialize() {}
func value() int            { return 1 }
func values() (int, error)  { return 1, nil }

func discardedFunc() {
	sync.OnceFunc(regressionInitialize)() // want "sync.OnceFunc wrapper is discarded after one call"
}

func discardedValue() int {
	return sync.OnceValue(value)() // want "sync.OnceValue wrapper is discarded after one call"
}

func discardedValues() (int, error) {
	return sync.OnceValues(values)() // want "sync.OnceValues wrapper is discarded after one call"
}

var regressionInitializeOnce = sync.OnceFunc(regressionInitialize)

// Package initialization already evaluates these expressions exactly once.
var initializedValue = sync.OnceValue(value)()
var initializedA, initializedError = sync.OnceValues(values)()

var repeatableFactory = func() int {
	return sync.OnceValue(value)() // want "sync.OnceValue wrapper is discarded after one call"
}

func retained() {
	regressionInitializeOnce()
}
