package goroutineownership

import (
	"context"
	"errors"
)

// This file covers producer completion through channel sends, distinguishing
// fully drained, cancellation-bounded, and adequately buffered producers.

func abandonedRepeatedSend() error {
	errs := make(chan error)
	go func() {
		errs <- errors.New("first")
		errs <- errors.New("second")
	}()
	return <-errs
}

func mutuallyExclusiveProducerSend(fail bool) error {
	errs := make(chan error)
	go func() {
		if fail {
			errs <- errors.New("failed")
			return
		}
		errs <- nil
	}()
	return <-errs
}

func abandonedCompetingSends() error {
	errs := make(chan error)
	go func() { errs <- errors.New("first") }()
	go func() { errs <- errors.New("second") }()
	return <-errs
}

func drainedSends() {
	errs := make(chan error)
	go func() {
		errs <- errors.New("first")
		errs <- errors.New("second")
	}()
	<-errs
	<-errs
}

func drainedSendsInLoop() {
	errs := make(chan error)
	go func() {
		defer close(errs)
		errs <- errors.New("first")
		errs <- errors.New("second")
	}()
	for range errs {
	}
}

func drainedSendsThroughSelects() {
	errs := make(chan error)
	go func() {
		errs <- errors.New("first")
		errs <- errors.New("second")
	}()
	select {
	case <-errs:
	}
	select {
	case <-errs:
	}
}

func oneSelectDoesNotDrainRepeatedSends() error {
	errs := make(chan error)
	go func() {
		errs <- errors.New("first")
		errs <- errors.New("second")
	}()
	select {
	case err := <-errs:
		return err
	}
}

func oneNonBlockingSelectDoesNotDrainRepeatedSends() error {
	errs := make(chan error)
	go func() {
		errs <- errors.New("first")
		errs <- errors.New("second")
	}()
	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

func cancellationAwareSend(ctx context.Context) error {
	errs := make(chan error)
	go func() {
		select {
		case errs <- errors.New("failed"):
		case <-ctx.Done():
		}
	}()
	return <-errs
}

func adequatelyBufferedSends() error {
	errs := make(chan error, 2)
	go func() {
		errs <- errors.New("first")
		errs <- errors.New("second")
	}()
	return <-errs
}

func sendTwice(errs chan<- error) {
	errs <- errors.New("first")
	errs <- errors.New("second")
}

func abandonedNamedProducer() error {
	errs := make(chan error)
	go sendTwice(errs)
	return <-errs
}
