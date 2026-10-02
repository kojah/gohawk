package returnlabels

// Both branch paths reach one return. Returning the exact completion channel
// hands observation to the caller; returning another channel does not.
func mergedReturn(choice bool) <-chan bool {
	done := make(chan bool)
	go func() { done <- true }()
	if choice {
		println("branch")
	}
	return done
}

func unrelatedReturn(choice bool) <-chan bool {
	done := make(chan bool)
	other := make(chan bool)
	go func() { done <- true }() // want "goroutine is not joined on every return path"
	if choice {
		println("branch")
	}
	return other
}
