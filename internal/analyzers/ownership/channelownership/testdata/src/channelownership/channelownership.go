package channelownership

//gohawk:example flagged
func consume(events chan int) {
	defer close(events) // want "do not close a channel received from caller"
	for range events {
	}
}

//gohawk:example end

//gohawk:example ok
func consumeSafely(events <-chan int) {
	for range events {
	}
}

//gohawk:example end

func startOwnedSignal() {
	done := make(chan struct{})
	go finishOwnedSignal(done)
}

func finishOwnedSignal(done chan struct{}) { close(done) }

func guardedSafeClose[T any](channel chan T) {
	select {
	case <-channel:
	default:
		close(channel)
	}
}

// publish sends one result. results is closed after publication.
func publish(results chan<- int) {
	results <- 1
	close(results)
}

// undocumentedClose does not promise ownership of results.
func undocumentedClose(results chan<- int) {
	close(results) // want "do not close a channel received from caller"
}

type boundedProducer struct{}

func (*boundedProducer) literal(files chan<- string) { close(files) }

func (*boundedProducer) glob(files chan<- string) { close(files) }

func collectWithBoundProducer(producer *boundedProducer, literal bool) {
	files := make(chan string)
	finder := producer.glob
	if literal {
		finder = producer.literal
	}
	finder(files)
}

type retainedBoundProducer struct{}

func (*retainedBoundProducer) produce(results chan<- int) {
	close(results) // want "do not close a channel received from caller"
}

func retainAfterBoundCall(producer *retainedBoundProducer) {
	results := make(chan int)
	produce := producer.produce
	produce(results)
	results <- 1
}

type storedBoundProducer struct{}

func (*storedBoundProducer) produce(results chan<- int) {
	close(results) // want "do not close a channel received from caller"
}

var storedBoundProducerCall func(chan<- int)

func storeBoundCall(producer *storedBoundProducer) {
	storedBoundProducerCall = producer.produce
}

type returnedBoundProducer struct{}

func (*returnedBoundProducer) produce(results chan<- int) {
	close(results) // want "do not close a channel received from caller"
}

func returnBoundCall(producer *returnedBoundProducer) func(chan<- int) {
	return producer.produce
}

type opaqueBoundProducer struct{}

func (*opaqueBoundProducer) produce(results chan<- int) {
	close(results) // want "do not close a channel received from caller"
}

func acceptOpaqueBoundCall(func(chan<- int)) {}

func passBoundCallToOpaqueHelper(producer *opaqueBoundProducer) {
	acceptOpaqueBoundCall(producer.produce)
}

type unsupportedBoundProducer struct{}

func (*unsupportedBoundProducer) produce(results chan<- int) (int, int) {
	close(results) // want "do not close a channel received from caller"
	return 0, 0
}

func useUnsupportedBoundCall(producer *unsupportedBoundProducer) {
	directResults := make(chan int)
	_, _ = producer.produce(directResults)

	boundResults := make(chan int)
	produce := producer.produce
	_, _ = produce(boundResults)
}

type packageBoundProducer struct{}

func (*packageBoundProducer) produce(results chan<- int) {
	close(results) // want "do not close a channel received from caller"
}

var packageBoundProducerCall = (&packageBoundProducer{}).produce

func callPackageBoundProducerDirectly(producer *packageBoundProducer) {
	results := make(chan int)
	producer.produce(results)
}

type methodExpressionProducer struct{}

func (*methodExpressionProducer) produce(results chan<- int) {
	close(results) // want "do not close a channel received from caller"
}

var storedMethodExpression = (*methodExpressionProducer).produce

func callMethodExpressionProducerDirectly(producer *methodExpressionProducer) {
	results := make(chan int)
	producer.produce(results)
}
