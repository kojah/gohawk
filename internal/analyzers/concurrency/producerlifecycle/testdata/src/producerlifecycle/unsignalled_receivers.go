package producerlifecycle

import "errors"

// Unsignalled receivers. A goroutine launched once that waits, on every
// path, on a channel only the function can send on or close blocks forever
// when the function returns without doing either, as a stop channel left
// open on an error return does. A range over the channel ends only on a
// close; a single receive takes a send or a close. A deferred close, a send
// on every path, and a worker that can stop waiting on something else are
// accepted.

func releaseWorker() {}

func prepareRun(fail bool) error {
	if fail {
		return errors.New("failed")
	}
	return nil
}

func stopNotClosedOnError(fail bool) error {
	stop := make(chan struct{})
	go func() { // want "goroutine blocks forever receiving from stop: the function can return without sending on or closing it"
		<-stop
		releaseWorker()
	}()
	if err := prepareRun(fail); err != nil {
		return err
	}
	close(stop)
	return nil
}

func rangeNotClosedOnError(items []int, fail bool) error {
	work := make(chan int)
	go func() { // want "goroutine blocks forever receiving from work: the function can return without closing it"
		for item := range work {
			_ = item
		}
	}()
	for _, item := range items {
		work <- item
	}
	if err := prepareRun(fail); err != nil {
		return err
	}
	close(work)
	return nil
}

func stopClosedByDefer(fail bool) error {
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		<-stop
		releaseWorker()
	}()
	return prepareRun(fail)
}

func stopSentOnEveryPath(fail bool) error {
	stop := make(chan struct{}, 1)
	go func() {
		<-stop
		releaseWorker()
	}()
	err := prepareRun(fail)
	stop <- struct{}{}
	return err
}

// A range is ended only by a close; sends alone leave it waiting, but the
// deferred close ends it on every return.
func rangeClosedByDefer(items []int) {
	work := make(chan int)
	defer close(work)
	go func() {
		for item := range work {
			_ = item
		}
	}()
	for _, item := range items {
		work <- item
	}
}

// The worker can stop waiting on another channel.
func workerSelectsOnQuit(quit <-chan struct{}, fail bool) error {
	stop := make(chan struct{})
	go func() {
		select {
		case <-stop:
		case <-quit:
		}
	}()
	if err := prepareRun(fail); err != nil {
		return err
	}
	close(stop)
	return nil
}
