package channelpolicy

type Event struct{}

func handle(Event) {}

//gohawk:example flagged
func consume(events chan Event) {
	defer close(events) // want "do not close a channel received from caller"
	for event := range events {
		handle(event)
	}
}

//gohawk:example end

//gohawk:example ok
func consumeSafely(events <-chan Event) {
	for event := range events {
		handle(event)
	}
}

//gohawk:example end
