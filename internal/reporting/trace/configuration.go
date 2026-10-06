package trace

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// Configuration owns the process-wide selectors, destinations and capture
// lifetime. Updates share the writer lock; capture callers must exclude other
// tracing analyses while their temporary sink is installed.

type settings struct {
	selectors map[string]bool
	candidate string
	writer    io.Writer
	file      *os.File
	timing    *os.File
	// sink, when set, receives each selected event instead of writer.
	sink func(Record)
}

var global = struct {
	sync.Mutex
	active atomic.Bool
	config settings
}{config: settings{writer: os.Stderr}}

// RegisterFlags adds global trace options to the analysis driver's flag set.
func RegisterFlags(flags *flag.FlagSet) {
	// x/tools owns the generic -trace flag for runtime tracing, so gohawk's
	// evidence flags use an explicit prefix and can coexist with the driver.
	flags.Var(optionValue{kind: optionSelectors}, "gohawk-trace", "emit JSONL evidence for comma-separated analyzers/checks, or all")
	flags.Var(
		optionValue{kind: optionCandidate}, "gohawk-trace-candidate",
		"limit evidence tracing to the proof of candidates whose position contains this path[:line]",
	)
	flags.Var(optionValue{kind: optionFile}, "gohawk-trace-file", "append trace JSONL to this file instead of stderr")
	flags.Var(
		optionValue{kind: optionTimingFile}, "gohawk-timing-file",
		"append one JSONL record per analyzer and package with wall time and allocation to this file",
	)
}

type optionKind uint8

const (
	optionSelectors optionKind = iota
	optionCandidate
	optionFile
	optionTimingFile
)

type optionValue struct{ kind optionKind }

func (value optionValue) String() string { return "" }

func (value optionValue) Set(raw string) error {
	global.Lock()
	defer global.Unlock()
	switch value.kind {
	case optionSelectors:
		selectors := make(map[string]bool)
		for selector := range strings.SplitSeq(raw, ",") {
			selector = strings.TrimSpace(selector)
			if selector == "" {
				return errors.New("trace selector must not be empty")
			}
			selectors[selector] = true
		}
		global.config.selectors = selectors
		global.active.Store(len(selectors) > 0)
	case optionCandidate:
		global.config.candidate = raw
	case optionFile:
		if raw == "" {
			return errors.New("trace file must not be empty")
		}
		// The trace destination is an explicit CLI argument, not a path derived
		// from analyzed source or another untrusted input.
		//nolint:gosec // User-selected output path is the intended interface.
		file, err := os.OpenFile(raw, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("open trace file: %w", err)
		}
		if global.config.file != nil {
			_ = global.config.file.Close()
		}
		global.config.file = file
		global.config.writer = file
	case optionTimingFile:
		if raw == "" {
			return errors.New("timing file must not be empty")
		}
		//nolint:gosec // User-selected output path is the intended interface.
		file, err := os.OpenFile(raw, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("open timing file: %w", err)
		}
		if global.config.timing != nil {
			_ = global.config.timing.Close()
		}
		global.config.timing = file
		timingActive.Store(true)
	default:
		return errors.New("unknown trace option")
	}
	return nil
}

// Capture hands every event the selectors choose to sink, in this process,
// limited to candidates whose position contains candidate when it is set,
// until the returned function restores the previous settings. It serves a
// reader that treats the trace as data, such as gohawk dump trace, and is not
// safe to use while another analysis in the process is tracing.
func Capture(selectors []string, candidate string, sink func(Record)) (restore func()) {
	global.Lock()
	defer global.Unlock()
	previous, wasActive := global.config, global.active.Load()
	chosen := make(map[string]bool, len(selectors))
	for _, selector := range selectors {
		chosen[selector] = true
	}
	global.config.selectors, global.config.candidate, global.config.sink = chosen, candidate, sink
	global.active.Store(len(chosen) > 0)
	return func() {
		global.Lock()
		defer global.Unlock()
		global.config = previous
		global.active.Store(wasActive)
	}
}
