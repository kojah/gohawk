package synchelpers

func Close(ch chan int)   { close(ch) }
func Send(ch chan int)    { ch <- 1 }
func Receive(ch chan int) { <-ch }
func ConditionalClose(ch chan int, yes bool) {
	if yes {
		close(ch)
	}
}
func AsyncClose(ch chan int)    { go func() { close(ch) }() }
func DeferredClose(ch chan int) { defer close(ch) }
func ForwardClose(ch chan int)  { DeferredClose(ch) }
func Ignore(ch chan int)        {}
var hook func(chan int)

func Unknown(ch chan int) { hook(ch) }
