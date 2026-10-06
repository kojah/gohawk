package helpers

import "producereffects"

// Hidden receives, unknown receivers, alternate participants, buffers, and
// other channels cannot be counted as an abandoned synchronous receiver.
func balanced() {
	ch := make(chan int)
	go func() { producereffects.Twice(ch) }()
	producereffects.DrainTwo(ch)
}
func deferredReceive() {
	ch := make(chan int)
	go func() { producereffects.Twice(ch) }()
	<-ch
	defer producereffects.Receive(ch)
}
func opaqueReceiver() {
	ch := make(chan int)
	go func() { ch <- 1; ch <- 2 }()
	<-ch
	producereffects.Opaque(ch)
}
func conditionalReceiver(yes bool) {
	ch := make(chan int)
	go func() { ch <- 1; ch <- 2 }()
	<-ch
	producereffects.Maybe(ch, yes)
}
func otherParticipant() {
	ch := make(chan int)
	go func() { producereffects.Twice(ch) }()
	go producereffects.Receive(ch)
	<-ch
}
func buffered() {
	ch := make(chan int, 2)
	go func() { producereffects.Twice(ch) }()
	<-ch
}
func external(ch chan int) {
	go func() { producereffects.Twice(ch) }()
	<-ch
}
func noObservedReceiver() {
	ch := make(chan int)
	go func() { producereffects.Twice(ch) }()
}
func separateChannels() {
	a, b := make(chan int), make(chan int)
	go func() { producereffects.Send(a); producereffects.Send(b) }()
	<-a
	<-b
}
func directWorkerHiddenDrain() {
	ch := make(chan int)
	go func() { ch <- 1; ch <- 2 }()
	producereffects.DrainTwo(ch)
}
