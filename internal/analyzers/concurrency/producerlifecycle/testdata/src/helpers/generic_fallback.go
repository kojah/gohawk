package helpers

// The opaque callback prevents a complete protocol summary. Generic fallback
// counts the sends in the source body, with its channel bound to this caller.
func genericBalancedWorker[T any](ch chan T, value T, opaque func()) {
	opaque()
	ch <- value
	ch <- value
}

func genericBalancedFallback(opaque func()) {
	ch := make(chan int)
	go genericBalancedWorker(ch, 1, opaque)
	<-ch
	<-ch
}

func genericExcessWorker[T any](ch chan T, value T, opaque func()) {
	opaque()
	ch <- value
	ch <- value // want "goroutine send can block"
}

func genericExcessFallback(opaque func()) {
	ch := make(chan int)
	go genericExcessWorker(ch, 1, opaque)
	<-ch
}
