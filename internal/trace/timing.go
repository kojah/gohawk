package trace

import (
	"encoding/json"
	"sync/atomic"
)

// Timing output has its own enable flag so drivers can avoid measurement
// overhead when disabled. It shares the configured destination lock and writes
// each timing record as one JSONL append, independently of evidence selection.

var timingActive atomic.Bool

// TimingEnabled reports whether analyzer timings are being recorded, so the
// driver can skip reading memory statistics when they are not.
func TimingEnabled() bool {
	return timingActive.Load()
}

// Timing is one analyzer run over one package.
type Timing struct {
	Package    string `json:"package"`
	Analyzer   string `json:"analyzer"`
	DurationNS int64  `json:"duration_ns"`
	AllocBytes uint64 `json:"alloc_bytes"`
}

// RecordTiming appends one timing record. Each record is written in one call
// so the file stays valid JSONL when go vet runs several analyzer processes
// against the same file.
func RecordTiming(timing Timing) {
	if !timingActive.Load() {
		return
	}
	encoded, err := json.Marshal(timing)
	if err != nil {
		return
	}
	encoded = append(encoded, '\n')
	global.Lock()
	defer global.Unlock()
	if global.config.timing != nil {
		_, _ = global.config.timing.Write(encoded)
	}
}
