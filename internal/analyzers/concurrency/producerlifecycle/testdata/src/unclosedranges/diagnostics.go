package unclosedranges

import (
	"context"
	"errors"
	"sync"
)

// run closes its output only on success, so a failed run leaves the consumer
// ranging forever.
type producer struct{ out chan int }

func (p *producer) run(fail bool) error {
	if fail {
		return errors.New("failed")
	}
	p.out <- 1
	close(p.out)
	return nil
}

func consumeProducer(fail bool) {
	p := &producer{out: make(chan int)}
	var wg sync.WaitGroup
	wg.Go(func() { _ = p.run(fail) })
	wg.Go(func() {
		for range p.out { // want "range can wait forever: run returns an error without closing the channel"
		}
	})
	wg.Wait()
}

// A panic inside the range crashes the program rather than ending the wait.
type checked struct{ out chan int }

func (c *checked) run(fail bool) error {
	if fail {
		return errors.New("failed")
	}
	close(c.out)
	return nil
}

func consumeChecked(fail bool) {
	c := &checked{out: make(chan int)}
	go func() { _ = c.run(fail) }()
	for v := range c.out { // want "range can wait forever: run returns an error without closing the channel"
		if v < 0 {
			panic("negative")
		}
	}
}

// Errors tested against nil, and a context's cause after its Done channel
// fires, are non-nil failures.
type streaming struct{ out chan int }

func receiveOne() (int, error) { return 0, nil }

func (s *streaming) run(ctx context.Context) error {
	for {
		value, err := receiveOne()
		if err != nil {
			return err
		}
		if value < 0 {
			break
		}
		select {
		case s.out <- value:
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
	close(s.out)
	return nil
}

func consumeStreaming(ctx context.Context) {
	s := &streaming{out: make(chan int)}
	go func() { _ = s.run(ctx) }()
	for range s.out { // want "range can wait forever: run returns an error without closing the channel"
	}
}
