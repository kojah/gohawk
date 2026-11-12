package lsp

import (
	"encoding/json"
	"io"
	"runtime"
	"time"
)

// An optional record of what the server did and when, one JSON object per
// line. It exists so a test harness can measure what a user feels, such as
// the time from an edit to the findings that answer it, without guessing from
// the screen. It records scheduling facts only and never changes them.

type eventLog struct {
	writer io.Writer
}

type event struct {
	Time string `json:"time"`
	// Event is one of document, run-started, run-finished, published, status.
	Event string `json:"event"`
	// Detail narrows the event: open, change or save for a document, and
	// package, importers or workspace for a run.
	Detail    string `json:"kind,omitempty"`
	Path      string `json:"path,omitempty"`
	Version   int    `json:"version,omitempty"`
	Run       int    `json:"run,omitempty"`
	Label     string `json:"label,omitempty"`
	Unsaved   bool   `json:"unsaved,omitempty"`
	Outcome   string `json:"outcome,omitempty"`
	ElapsedMS int64  `json:"elapsedMs,omitempty"`
	PeakBytes int64  `json:"peakBytes,omitempty"`
	Findings  *int   `json:"findings,omitempty"`
	// ServerBytes and Goroutines describe the server process itself when a
	// run finishes. They should stay flat over a session: the server holds
	// bookkeeping, not analysis state.
	ServerBytes uint64 `json:"serverBytes,omitempty"`
	Goroutines  int    `json:"goroutines,omitempty"`
	State       string `json:"state,omitempty"`
	Text        string `json:"text,omitempty"`
}

func (l *eventLog) record(next event) {
	if l == nil || l.writer == nil {
		return
	}
	next.Time = time.Now().UTC().Format(time.RFC3339Nano)
	encoded, err := json.Marshal(next)
	if err != nil {
		return
	}
	// The log is best-effort: a full disk must not stop checks.
	_, _ = l.writer.Write(append(encoded, '\n'))
}

// enabled lets a caller skip work that only feeds the log.
func (l *eventLog) enabled() bool {
	return l != nil && l.writer != nil
}

// serverFootprint returns the memory the Go runtime has obtained from the
// operating system for this process, and its goroutine count.
func serverFootprint() (uint64, int) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.Sys, runtime.NumGoroutine()
}

func (k runKind) String() string {
	switch k {
	case runPackage:
		return "package"
	case runImporters:
		return "importers"
	default:
		return "workspace"
	}
}

func (o runOutcome) String() string {
	switch o {
	case outcomeCompleted:
		return "completed"
	case outcomeBuildFailed:
		return "build-failed"
	case outcomeCanceled:
		return "canceled"
	case outcomeTimedOut:
		return "timed-out"
	case outcomeMemoryExceeded:
		return "memory-exceeded"
	default:
		return "failed"
	}
}
