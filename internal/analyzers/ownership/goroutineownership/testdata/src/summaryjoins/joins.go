package summaryjoins

import (
	"sync"
	"synchelpers"
)

// These must be classified as honored, not merely suppressed as opaque.
// A helper's receive observes an already-proven completion signal.
func importedReceive() {
	done := make(chan struct{})
	go func() { defer close(done) }()
	synchelpers.Receive(done)
}
func importedWait() {
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done() }()
	synchelpers.Wait(&group)
}
func deferredReceive() {
	done := make(chan struct{})
	go func() { defer close(done) }()
	defer synchelpers.Forward(done)
}
func branchAwareWrapper(ch <-chan struct{}, verbose bool) {
	if verbose {
		println("waiting")
	}
	synchelpers.Receive(ch)
}
func branchAwareJoin(verbose bool) {
	done := make(chan struct{})
	go func() { defer close(done) }()
	branchAwareWrapper(done, verbose)
}
func opaqueReceive() {
	done := make(chan struct{})
	go func() { defer close(done) }()
	synchelpers.Unknown(done)
}
func conditionalReceive(yes bool) {
	done := make(chan struct{})
	go func() { defer close(done) }()
	synchelpers.Maybe(done, yes)
}
func asynchronousReceive() {
	done := make(chan struct{})
	go func() { defer close(done) }()
	go synchelpers.Receive(done)
}
func differentSignal() {
	done, other := make(chan struct{}), make(chan struct{})
	go func() { defer close(done) }() // want "goroutine is not joined"
	synchelpers.Receive(other)
}
func differentArgument() {
	done, other := make(chan struct{}), make(chan struct{})
	go func() { defer close(done) }()
	synchelpers.Other(done, other) // Unknown use of done must not become a join.
}
func missingPath(yes bool) {
	done := make(chan struct{})
	go func() { defer close(done) }() // want "goroutine is not joined"
	if yes {
		synchelpers.Receive(done)
	}
}
