package producerlifecycle

import "log"

// A repeated-send loop does not establish its runtime cardinality. Infinite
// producer loops are an accepted coverage gap rather than a heuristic warning.
func oneMapEntry() {
	results := make(chan int)
	values := map[string]int{"only": 1}
	go func() {
		for _, value := range values {
			results <- value
		}
	}()
	<-results
}

func atMostOneSuccessfulResult(errors <-chan error) {
	result := make(chan error)
	go func() {
		succeeded := false
		for err := range errors {
			if err == nil && !succeeded {
				succeeded = true
				result <- nil
			}
		}
		if !succeeded {
			result <- nil
		}
	}()
	<-result
}

func processTerminatesAfterFirstResult() {
	results := make(chan error)
	go func() { results <- nil }()
	go func() { results <- nil }()
	log.Fatal(<-results)
}

// A go statement in a loop starts as many producers as the loop runs, which
// may be zero or one. One receive is too few only when the loop runs twice.
func producerPerItem(items []string) string {
	results := make(chan string)
	for _, item := range items {
		go func() { results <- item }() // producer started per item
	}
	return <-results
}
