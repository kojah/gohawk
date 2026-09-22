package doubleclose

import "synchelpers"

func direct(ch chan int) {
	close(ch)
	close(ch) // want "close follows close of channel"
}

func alias(ch chan int) {
	other := ch
	close(ch)
	close(other) // want "close follows close of channel"
}

func field() {
	box := struct{ ch chan int }{make(chan int)}
	close(box.ch)
	close(box.ch) // want "close follows close of channel"
}

func imported(ch chan int) {
	synchelpers.Close(ch)
	synchelpers.ForwardClose(ch) // want "close follows close of channel"
}

func pair(a, b chan int) { close(a); close(b) }

func sameCall(ch chan int) {
	pair(ch, ch) // want "close follows close of channel"
}
