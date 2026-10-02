package notificationpromises

// Conditional notification and registration do not promise a completion event
// on every worker return. Correlating arbitrary caller and worker guards is
// outside this proof; genuine omissions in those forms can remain unreported.
func optionalClose(flag bool) {
	done := make(chan struct{})
	go func() {
		defer func() {
			if flag {
				close(done)
			}
		}()
	}()
	if flag {
		<-done
	}
}

func optionalSend(flag bool) {
	done := make(chan bool)
	go func() {
		defer func() {
			if flag {
				done <- true
			}
		}()
	}()
	if flag {
		<-done
	}
}

func optionalRegistration(flag bool) {
	done := make(chan struct{})
	go func() {
		if flag {
			defer func() { close(done) }()
		}
	}()
	if flag {
		<-done
	}
}

func nestedProgress() {
	done := make(chan bool)
	go func() { func() { done <- true; blockingWork() }() }()
}

func outerProgress() {
	done := make(chan bool)
	go func() { func() { done <- true }(); blockingWork() }()
}

func detachedClose(flag bool) {
	done := make(chan struct{})
	go func() { defer func() { go close(done) }() }()
	if flag {
		<-done
	}
}

func blockingWork() { select {} }

func deferredMissing() {
	done := make(chan bool)
	go func() { defer func() { done <- true }() }() // want "goroutine is not joined on every return path"
}

func synchronousMissing() {
	done := make(chan bool)
	go func() { func() { done <- true }() }() // want "goroutine is not joined on every return path"
}

func deferredJoined() {
	done := make(chan bool)
	go func() { defer func() { done <- true }() }()
	<-done
}

func synchronousJoined() {
	done := make(chan bool)
	go func() { func() { done <- true }() }()
	<-done
}

func alternateMissing(failed bool) {
	done := make(chan bool)
	go func() { // want "goroutine is not joined on every return path"
		if failed {
			close(done)
		} else {
			done <- true
		}
	}()
}
