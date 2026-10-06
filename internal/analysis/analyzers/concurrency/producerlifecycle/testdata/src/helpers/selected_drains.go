package helpers

func selectedDrain(yes bool) {
	ch := make(chan int)
	go func() { ch <- 1; ch <- 2 }()
	<-ch
	drain := func() { <-ch }
	if yes {
		drain = func() { <-ch }
	}
	drain()
}

// Callback selection alone does not hide a receiver: these closures capture
// only unrelated scalar storage. The excess second send is still observable.
func selectedUnrelatedCallback(yes bool) {
	ch := make(chan int)
	go func() {
		ch <- 1
		ch <- 2 // want "goroutine send can block"
	}()
	<-ch
	n := 0
	callback := func() { n++ }
	if yes {
		callback = func() { n += 2 }
	}
	callback()
}
