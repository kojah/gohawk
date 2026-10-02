package readlockpaths

import "sync"

// The two short-circuit predecessors reach the same write with different
// branch evidence. That is one diagnostic, rather than one per flow state.
type state struct {
	mu     sync.RWMutex
	other  sync.RWMutex
	writer sync.Mutex
	cursor *int
	count  int
}

func (s *state) prepare() {
	if s.cursor == nil || *s.cursor == 0 {
		s.mu.RLock()
		defer s.mu.RUnlock()
		s.count++           // want "write while only the read lock `s\\.mu` is held"
		s.cursor = new(int) // want "write while only the read lock `s\\.mu` is held"
	}
}

// An unknown writer-guard path must not hide the unguarded path's report.
func (s *state) maybeGuarded(guard bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if guard {
		s.writer.Lock()
		defer s.writer.Unlock()
	}
	s.count++ // want "write while only the read lock `s\\.mu` is held"
}

// Different read-lock identities at one write retain their separate reports.
func (s *state) differentLocks(first bool) {
	if first {
		s.mu.RLock()
		defer s.mu.RUnlock()
	} else {
		s.other.RLock()
		defer s.other.RUnlock()
	}
	s.count++ // want "write while only the read lock `s\\.mu` is held" "write while only the read lock `s\\.other` is held"
}
