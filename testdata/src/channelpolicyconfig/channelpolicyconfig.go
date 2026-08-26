package channelpolicyconfig

func configured(events chan int) {
	queue := make(chan int, 8)
	close(events)
	close(queue)
	queue <- 1
}
