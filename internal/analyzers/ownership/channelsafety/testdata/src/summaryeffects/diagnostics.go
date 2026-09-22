package summaryeffects

import "synchelpers"

func importedClose(ch chan int) {
	synchelpers.Close(ch)
	ch <- 1 // want "send follows close"
}
func importedSend(ch chan int) {
	close(ch)
	synchelpers.Send(ch) // want "send follows close"
}
func importedBoth(ch chan int) {
	synchelpers.Close(ch)
	synchelpers.Send(ch) // want "send follows close"
}
func importedDeferredClose(ch chan int) {
	synchelpers.ForwardClose(ch)
	synchelpers.Send(ch) // want "send follows close"
}
func localClose(ch chan int) { close(ch) }
func localSend(ch chan int)  { ch <- 1 }
func localHelpers(ch chan int) {
	localClose(ch)
	localSend(ch) // want "send follows close"
}
