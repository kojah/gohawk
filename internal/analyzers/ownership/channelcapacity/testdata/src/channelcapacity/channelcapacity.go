package channelcapacity

//gohawk:example flagged
func unexplainedQueue() {
	_ = make(chan int, 3) // want "channel capacity 3 requires a bounded rationale"
}

//gohawk:example end

//gohawk:example ok
func boundedQueue() {
	// Bounded: one slot per fixed worker.
	_ = make(chan int, 3)
}

//gohawk:example end

func runtimeBound(size int) { _ = make(chan int, size) }
