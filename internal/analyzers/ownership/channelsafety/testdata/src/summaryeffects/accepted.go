package summaryeffects

import "synchelpers"

// A summary must preserve exact channel identity, synchronous execution, and
// defer timing. Unsupported branches and opaque calls provide no close proof.
func distinctChannels() {
	a, b := make(chan int, 1), make(chan int, 1)
	synchelpers.Close(a)
	synchelpers.Send(b)
}
func sendBeforeClose(ch chan int) { synchelpers.Send(ch); synchelpers.Close(ch) }
func deferredClose(ch chan int)   { defer synchelpers.Close(ch); synchelpers.Send(ch) }
func asyncClose(ch chan int)      { go synchelpers.Close(ch); synchelpers.Send(ch) }
func hiddenSpawn(ch chan int)     { synchelpers.AsyncClose(ch); synchelpers.Send(ch) }
func conditionalClose(ch chan int, yes bool) {
	synchelpers.ConditionalClose(ch, yes)
	synchelpers.Send(ch)
}
func opaqueClose(ch chan int)       { synchelpers.Unknown(ch); synchelpers.Send(ch) }
func receiveAfterClose(ch chan int) { synchelpers.Close(ch); synchelpers.Receive(ch) }
func unusedSummary(ch chan int)     { synchelpers.Ignore(ch); synchelpers.Send(ch) }
func reassignedChannel() {
	ch := make(chan int, 1)
	synchelpers.Close(ch)
	ch = make(chan int, 1)
	synchelpers.Send(ch)
}
func exclusiveBranches(ch chan int, yes bool) {
	if yes {
		synchelpers.Close(ch)
	} else {
		synchelpers.Send(ch)
	}
}
func loopLocal() {
	for range 2 {
		ch := make(chan int, 1)
		synchelpers.Send(ch)
		synchelpers.Close(ch)
	}
}
