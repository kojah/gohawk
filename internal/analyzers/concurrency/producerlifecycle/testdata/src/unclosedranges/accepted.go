package unclosedranges

import (
	"context"
	"errors"
	"sync"
)

// A deferred close runs on every return, the error returns included.
type deferred struct{ out chan int }

func (d *deferred) run(fail bool) error {
	defer close(d.out)
	if fail {
		return errors.New("failed")
	}
	d.out <- 1
	return nil
}

func consumeDeferred(fail bool) {
	d := &deferred{out: make(chan int)}
	var wg sync.WaitGroup
	wg.Go(func() { _ = d.run(fail) })
	for range d.out {
	}
	wg.Wait()
}

// Closing before each return covers the error path too.
type everyReturn struct{ out chan int }

func (e *everyReturn) run(fail bool) error {
	if fail {
		close(e.out)
		return errors.New("failed")
	}
	e.out <- 1
	close(e.out)
	return nil
}

func consumeEveryReturn(fail bool) {
	e := &everyReturn{out: make(chan int)}
	go func() { _ = e.run(fail) }()
	for range e.out {
	}
}

// Checking the error before ranging never waits on a failed run.
type sequential struct{ out chan int }

func (s *sequential) run(fail bool) error {
	if fail {
		return errors.New("failed")
	}
	s.out = make(chan int, 1)
	s.out <- 1
	close(s.out)
	return nil
}

func consumeSequential(fail bool) error {
	s := &sequential{}
	if err := s.run(fail); err != nil {
		return err
	}
	for range s.out {
	}
	return nil
}

// A retried run can close the channel on a later attempt.
type retried struct{ out chan int }

func (r *retried) run(fail bool) error {
	if fail {
		return errors.New("failed")
	}
	close(r.out)
	return nil
}

func consumeRetried(attempts []bool) {
	r := &retried{out: make(chan int)}
	go func() {
		for _, fail := range attempts {
			if r.run(fail) == nil {
				return
			}
		}
	}()
	for range r.out {
	}
}

// A second closer may close the channel after a failed run.
type twoClosers struct{ out chan int }

func (t *twoClosers) run(fail bool) error {
	if fail {
		return errors.New("failed")
	}
	close(t.out)
	return nil
}

func (t *twoClosers) abort() { close(t.out) }

func consumeTwoClosers(fail bool) {
	t := &twoClosers{out: make(chan int)}
	go func() {
		if t.run(fail) != nil {
			t.abort()
		}
	}()
	for range t.out {
	}
}

// Returning nil without closing is a step, not a failure: a later call closes.
type stepping struct{ out chan int }

func (s *stepping) step(done bool) error {
	if !done {
		return nil
	}
	close(s.out)
	return nil
}

func consumeStepping(done bool) {
	s := &stepping{out: make(chan int)}
	go func() { _ = s.step(done) }()
	for range s.out {
	}
}

// A range that can break does not wait only for the close.
type breaking struct{ out chan int }

func (b *breaking) run(fail bool) error {
	if fail {
		return errors.New("failed")
	}
	close(b.out)
	return nil
}

func consumeBreaking(fail bool) {
	b := &breaking{out: make(chan int)}
	go func() { _ = b.run(fail) }()
	for v := range b.out {
		if v < 0 {
			break
		}
	}
}

// A channel handed out may be closed by whoever receives it.
type escaping struct{ out chan int }

func (e *escaping) channel() chan int { return e.out }

func (e *escaping) run(fail bool) error {
	if fail {
		return errors.New("failed")
	}
	close(e.out)
	return nil
}

func consumeEscaping(fail bool) {
	e := &escaping{out: make(chan int)}
	go func() { _ = e.run(fail) }()
	for range e.out {
	}
}

// A supplied channel may have other closers.
type supplied struct{ out chan int }

func (s *supplied) run(fail bool) error {
	if fail {
		return errors.New("failed")
	}
	close(s.out)
	return nil
}

func consumeSupplied(out chan int, fail bool) {
	s := &supplied{out: out}
	go func() { _ = s.run(fail) }()
	for range s.out {
	}
}

// Another package can close an exported field of a library type.
type Exported struct{ Out chan int }

func (e *Exported) run(fail bool) error {
	if fail {
		return errors.New("failed")
	}
	close(e.Out)
	return nil
}

func consumeExported(fail bool) {
	e := &Exported{Out: make(chan int)}
	go func() { _ = e.run(fail) }()
	for range e.Out {
	}
}

// Running one object and ranging another relates nothing.
type pair struct{ out chan int }

func (p *pair) run(fail bool) error {
	if fail {
		return errors.New("failed")
	}
	close(p.out)
	return nil
}

func consumeOther(fail bool) {
	first := &pair{out: make(chan int)}
	second := &pair{out: make(chan int)}
	go func() { _ = first.run(fail) }()
	for range second.out {
	}
}

// A failure that panics never returns without the close.
type panicking struct{ out chan int }

func (p *panicking) run(fail bool) error {
	if fail {
		panic("failed")
	}
	close(p.out)
	return nil
}

func consumePanicking(fail bool) {
	p := &panicking{out: make(chan int)}
	go func() { _ = p.run(fail) }()
	for range p.out {
	}
}

// A context's error read without its Done channel firing may be nil, so the
// unclosed return is not a proven failure.
type polling struct{ out chan int }

func (p *polling) run(ctx context.Context) error {
	if ready() {
		return ctx.Err()
	}
	close(p.out)
	return nil
}

func ready() bool { return true }

func consumePolling(ctx context.Context) {
	p := &polling{out: make(chan int)}
	go func() { _ = p.run(ctx) }()
	for range p.out {
	}
}
