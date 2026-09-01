package testfiles

import "testing"

func TestBufferedFixture(t *testing.T) {
	results := make(chan int, 3)
	for value := range 3 {
		results <- value
	}
	for range 3 {
		<-results
	}
}

func closeBorrowedInTest(events chan int) {
	close(events) // want "do not close a channel received from caller"
}
