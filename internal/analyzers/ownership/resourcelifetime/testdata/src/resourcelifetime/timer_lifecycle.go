package resourcelifetime

// Accepted gap: a launched literal that captures the ticker and stops it only
// conditionally is an opaque consumption, not a proven leak.

import (
	"context"
	"sync"
	"time"
)

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

// sync.WaitGroup.Go launches its argument on a new goroutine, so a ticker
// stopped on every return of that closure is settled exactly like one stopped
// in a go statement.
func tickerStoppedInWaitGroupGo(ctx context.Context, group *sync.WaitGroup) {
	ticker := time.NewTicker(time.Second)
	group.Go(func() {
		defer ticker.Stop()
		<-ctx.Done()
	})
}

func tickerLeakedInWaitGroupGo(ctx context.Context, group *sync.WaitGroup) {
	ticker := time.NewTicker(time.Second) // want "owned resource from time.NewTicker is not released on every return path"
	group.Go(func() {
		<-ctx.Done()
		<-ticker.C
	})
}

// A loop that ranges over the ticker's own channel exits only when that
// channel closes, which time documents never happens, so the return after
// the loop is unreachable and the ticker is not reported.
func tickerRangedForever(interval time.Duration, work func()) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		work()
	}
}

// A break makes the exit reachable, and the ticker is then leaked.
func tickerRangedUntilDone(interval time.Duration, done func() bool) {
	ticker := time.NewTicker(interval) // want "owned resource from time.NewTicker is not released on every return path"
	for range ticker.C {
		if done() {
			break
		}
	}
}
