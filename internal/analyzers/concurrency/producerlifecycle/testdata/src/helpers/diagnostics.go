package helpers

import "producereffects"

func hiddenSends() {
	ch := make(chan int)
	go func() {
		producereffects.Send(ch)
		producereffects.Send(ch) // want "goroutine send can block"
	}()
	<-ch
}
func importedWorker() {
	ch := make(chan int)
	go producereffects.Twice(ch) // want "goroutine send can block"
	<-ch
}
func hiddenReceiver() {
	ch := make(chan int)
	go func() {
		ch <- 1
		ch <- 2 // want "goroutine send can block"
	}()
	producereffects.Receive(ch)
}
