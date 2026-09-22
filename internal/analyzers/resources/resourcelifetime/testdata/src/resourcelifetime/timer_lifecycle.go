package resourcelifetime

// Channel timers need no Stop for reclamation under Go 1.23+ semantics.
// Accepted gap: legacy main-module/GODEBUG settings and retained background
// workers are not inferred from a missing Stop in this package-local pass.

import (
	"os"
	"time"
)

func collectableTimer() {
	timer := time.NewTimer(time.Hour)
	_ = timer.C
}

func collectableTicker() {
	ticker := time.NewTicker(time.Second)
	_ = ticker.C
}

func abandonedTimerSelect(done <-chan struct{}) {
	timer := time.NewTimer(time.Hour)
	select {
	case <-timer.C:
	case <-done:
	}
}

func tickerBreak(done func() bool) {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		if done() {
			break
		}
	}
}

// Collectable timers do not exempt another resource acquired beside them.
func timerWithLeakedFile(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	timer := time.NewTimer(time.Hour)
	_ = timer.C
	_ = file
	return nil
}
