// Double-close proofs stay inside one basic block. Cross-block closes,
// caller defers, asynchronous closes, and unknown helpers are not modeled.
package doubleclose

import (
	"sync"
	"synchelpers"
)

func distinct() { a, b := make(chan int), make(chan int); close(a); close(b) }
func replaced() { ch := make(chan int); close(ch); ch = make(chan int); close(ch) }
func exclusive(ch chan int, yes bool) {
	if yes {
		close(ch)
	} else {
		close(ch)
	}
}
func freshLoop(n int) {
	for range n {
		ch := make(chan int)
		close(ch)
	}
}
func once(ch chan int)                  { var once sync.Once; f := func() { close(ch) }; once.Do(f); once.Do(f) }
func deferred(ch chan int)              { defer close(ch) }
func launched(ch chan int)              { go func() { close(ch) }() }
func conditional(ch chan int, yes bool) { synchelpers.ConditionalClose(ch, yes); close(ch) }
func opaque(ch chan int)                { synchelpers.Unknown(ch); close(ch) }
func nilChannel()                       { var ch chan int; close(ch); close(ch) }
func changedField() {
	box := struct{ ch chan int }{make(chan int)}
	close(box.ch)
	box.ch = make(chan int)
	close(box.ch)
}
func independentSelections(a, b chan int, chooseA, chooseB bool) {
	x, y := a, b
	if chooseA {
		x = b
	}
	if chooseB {
		y = a
	}
	close(x)
	close(y)
}
