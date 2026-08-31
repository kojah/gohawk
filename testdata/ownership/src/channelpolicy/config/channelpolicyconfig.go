package channelpolicyconfig

func configured() {
	_ = make(chan int, 8)
}
