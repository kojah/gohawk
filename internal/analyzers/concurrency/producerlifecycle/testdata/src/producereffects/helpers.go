package producereffects

func Send(ch chan int)     { ch <- 1 }
func Receive(ch chan int)  { <-ch }
func Twice(ch chan int)    { ch <- 1; ch <- 2 }
func DrainTwo(ch chan int) { <-ch; <-ch }
func Maybe(ch chan int, yes bool) {
	if yes {
		<-ch
	}
}

var hook func(chan int)

func Opaque(ch chan int) { hook(ch) }
