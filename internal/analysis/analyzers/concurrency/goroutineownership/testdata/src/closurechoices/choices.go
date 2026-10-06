package closurechoices

// A consumer selected from closure alternatives is an opaque participant, not
// proof that the producer joins. An unrelated choice preserves the diagnostic.
func merged(flag bool, workers int) {
	done := make(chan int)
	other := make(chan int)
	go func() { close(done) }()
	var consumer func()
	if flag {
		consumer = func() { <-done }
	} else {
		consumer = func() { <-other }
	}
	for range workers {
		go consumer()
	}
}

func unrelated(flag bool) {
	done := make(chan int)
	other := make(chan int)
	go func() { close(done) }() // want "goroutine is not joined"
	var consumer func()
	if flag {
		consumer = func() { <-other }
	} else {
		consumer = func() { println(1) }
	}
	go consumer()
	if flag {
		<-done
	}
}

func runner(func())

func callback(flag bool) {
	done := make(chan int)
	other := make(chan int)
	go func() { close(done) }()
	var consumer func()
	if flag {
		consumer = func() { <-done }
	} else {
		consumer = func() { <-other }
	}
	runner(consumer)
}

// A converted callable remains outside this query's transparent forms.
// This is not a safety claim about the worker or a guessed target resolution.
type converted func()

func wrapper() {
	done := make(chan int)
	go func() { close(done) }() // want "goroutine is not joined"
	consumer := converted(func() { <-done })
	go consumer()
}
