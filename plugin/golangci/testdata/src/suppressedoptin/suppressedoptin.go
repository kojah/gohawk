package suppressedoptin

// stopped-loop-send is experimental, so the default profile must not report
// the send in Schedule, which blocks forever once Stop has ended run.
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

func (s *scheduler) Schedule(v int) { s.add <- v }

func (s *scheduler) Stop() { s.stop <- struct{}{} }
