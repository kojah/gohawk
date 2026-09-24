package serviceloops

import "context"

// The gocronx-team/cron shape: after Stop, run has returned and nothing
// receives from add, so Schedule blocks forever.
type scheduler struct {
	add  chan int
	stop chan struct{}
}

func newScheduler() *scheduler {
	s := &scheduler{add: make(chan int), stop: make(chan struct{})}
	go s.run()
	return s
}

func (s *scheduler) run() {
	for {
		select {
		case <-s.add:
		case <-s.stop:
			return
		}
	}
}

func (s *scheduler) Schedule(v int) {
	s.add <- v // want "send can block forever after the service loop receiving it returns"
}

func (s *scheduler) Stop() { s.stop <- struct{}{} }

// The ErnestK/MCPSprut shape with an unbuffered channel: the loop returns when
// its context is cancelled.
type batcher struct{ updates chan string }

func newBatcher(ctx context.Context) *batcher {
	b := &batcher{updates: make(chan string)}
	go b.loop(ctx)
	return b
}

func (b *batcher) loop(ctx context.Context) {
	for {
		select {
		case <-b.updates:
		case <-ctx.Done():
			return
		}
	}
}

func (b *batcher) Submit(update string) {
	b.updates <- update // want "send can block forever after the service loop receiving it returns"
}

// Names carry no meaning: here the data channel is called stop and the
// shutdown channel is called add.
type misleadingNames struct {
	stop chan int
	add  chan struct{}
}

func newMisleadingNames() *misleadingNames {
	m := &misleadingNames{stop: make(chan int), add: make(chan struct{})}
	go m.run()
	return m
}

func (m *misleadingNames) run() {
	for {
		select {
		case <-m.stop:
		case <-m.add:
			return
		}
	}
}

func (m *misleadingNames) Send(v int) {
	m.stop <- v // want "send can block forever after the service loop receiving it returns"
}

func (m *misleadingNames) Close() { close(m.add) }

// A branch on an unrelated argument is not a lifecycle guard.
func (s *scheduler) ScheduleIfPositive(v int) {
	if v <= 0 {
		return
	}
	s.add <- v // want "send can block forever after the service loop receiving it returns"
}
