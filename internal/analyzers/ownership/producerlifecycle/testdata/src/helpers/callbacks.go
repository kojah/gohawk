package helpers

import "producereffects"

func registeredReceiver(register func(func())) {
	ch := make(chan int)
	go producereffects.Twice(ch)
	register(func() { <-ch })
	<-ch
}
