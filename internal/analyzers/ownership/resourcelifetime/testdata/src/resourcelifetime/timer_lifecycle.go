package resourcelifetime

import "time"

func tickerOwnedByWorker(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	go func() {
		defer ticker.Stop()
		<-stop
	}()
}

func tickerStoppedOnWorkerExit(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	go func() {
		select {
		case <-ticker.C:
		case <-stop:
		}
		ticker.Stop()
	}()
}

func tickerConditionallyStoppedByWorker(stop <-chan struct{}, cleanup bool) {
	ticker := time.NewTicker(time.Second) // want "owned resource from time.NewTicker is not released on every return path"
	go func() {
		<-stop
		if cleanup {
			ticker.Stop()
		}
	}()
}

func leakedTimer() {
	timer := time.NewTimer(time.Second) // want "owned resource from time.NewTimer is not released on every return path"
	_ = timer
}

func stoppedTimer() {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
}

func consumedTimer() {
	timer := time.NewTimer(time.Second)
	<-timer.C
}

func consumedTimerInSelect() {
	timer := time.NewTimer(time.Second)
	select {
	case <-timer.C:
	default:
	}
}

func unrelatedReceiveDoesNotConsumeTimer(events <-chan time.Time) {
	timer := time.NewTimer(time.Second) // want "owned resource from time.NewTimer is not released on every return path"
	<-events
	_ = timer
}

func timerCommand() func() {
	timer := time.NewTimer(time.Second)
	return func() {
		<-timer.C
		timer.Stop()
	}
}

func stoppedOrConsumedTimer(events <-chan int) {
	timer := time.NewTimer(time.Second)
	done := false
	for !done {
		select {
		case <-events:
			done = true
		case <-timer.C:
			return
		}
	}
	if !timer.Stop() {
		<-timer.C
	}
}

func stoppedOrConsumedTimerAfterSeveralEvents(events <-chan string) {
	identities := make(map[string]bool, 2)
	timedOut := false
	timer := time.NewTimer(time.Second)
	for len(identities) < 2 && !timedOut {
		select {
		case identity := <-events:
			identities[identity] = true
		case <-timer.C:
			timedOut = true
		}
	}
	if !timedOut && !timer.Stop() {
		<-timer.C
	}
}

func partiallyHandledTimer(stop, receive bool) {
	timer := time.NewTimer(time.Second) // want "owned resource from time.NewTimer is not released on every return path"
	if stop {
		timer.Stop()
	}
	if receive {
		<-timer.C
	}
}
