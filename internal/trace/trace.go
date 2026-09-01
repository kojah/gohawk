// Package trace provides opt-in structured evidence tracing for analyzers.
package trace

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/tools/go/analysis"
)

// Outcome is the result of an evidence decision.
type Outcome string

const (
	OutcomeObserved Outcome = "observed"
	OutcomeAccepted Outcome = "accepted"
	OutcomeRejected Outcome = "rejected"
	OutcomeUnknown  Outcome = "unknown"
)

// Event is one stable analyzer evidence decision. Details should contain only
// compact identifiers and counts; tracing is diagnostic metadata, not an SSA
// serialization format.
type Event struct {
	Analyzer string
	Check    string
	Phase    string
	Reason   string
	Outcome  Outcome
	Pos      token.Pos
	Function string
	Details  map[string]string
}

type record struct {
	Analyzer string            `json:"analyzer"`
	Check    string            `json:"check,omitempty"`
	Phase    string            `json:"phase"`
	Reason   string            `json:"reason"`
	Outcome  Outcome           `json:"outcome"`
	Position string            `json:"position,omitempty"`
	Function string            `json:"function,omitempty"`
	Details  map[string]string `json:"details,omitempty"`
}

type settings struct {
	selectors map[string]bool
	source    string
	function  string
	writer    io.Writer
	file      *os.File
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
	flags.Var(optionValue{kind: optionSource}, "gohawk-trace-source", "limit evidence tracing to positions containing this path[:line]")
	flags.Var(optionValue{kind: optionFunction}, "gohawk-trace-function", "limit evidence tracing to functions containing this text")
	flags.Var(optionValue{kind: optionFile}, "gohawk-trace-file", "append trace JSONL to this file instead of stderr")
}

type optionKind uint8

const (
	optionSelectors optionKind = iota
	optionSource
	optionFunction
	optionFile
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
	case optionSource:
		global.config.source = raw
	case optionFunction:
		global.config.function = raw
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
	default:
		return errors.New("unknown trace option")
	}
	return nil
}

// Enabled reports whether an event for analyzer or check can pass the selector
// filter. Callers use it before allocating diagnostic details on the hot path.
func Enabled(analyzer, check string) bool {
	if !global.active.Load() {
		return false
	}
	global.Lock()
	defer global.Unlock()
	return global.config.selectors["all"] || global.config.selectors[analyzer] || check != "" && global.config.selectors[check]
}

// DiagnosticEvent describes one source-level analyzer decision. Named fields
// keep the analyzer identity, evidence phase, and reason from being transposed.
type DiagnosticEvent struct {
	Analyzer   string
	Phase      string
	Reason     string
	Outcome    Outcome
	Diagnostic analysis.Diagnostic
}

// EmitDiagnostic records a source-level analyzer decision using the same
// schema as deeper SSA evidence. Function names are resolved only while the
// corresponding trace selector is enabled.
func EmitDiagnostic(pass *analysis.Pass, event DiagnosticEvent) {
	if !Enabled(event.Analyzer, event.Diagnostic.Category) {
		return
	}
	details := map[string]string{}
	if event.Diagnostic.Message != "" {
		details["message"] = event.Diagnostic.Message
	}
	Emit(pass, Event{
		Analyzer: event.Analyzer,
		Check:    event.Diagnostic.Category,
		Phase:    event.Phase,
		Reason:   event.Reason,
		Outcome:  event.Outcome,
		Pos:      event.Diagnostic.Pos,
		Function: sourceFunction(pass, event.Diagnostic.Pos),
		Details:  details,
	})
}

func sourceFunction(pass *analysis.Pass, position token.Pos) string {
	if pass == nil || !position.IsValid() {
		return ""
	}
	for _, file := range pass.Files {
		if position < file.Pos() || position > file.End() {
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Pos() <= position && position <= function.End() {
				return function.Name.Name
			}
		}
	}
	return ""
}

// Emit writes event as one JSON object when it matches the configured filters.
// Each event is marshaled and written in one call so append-mode trace files
// remain usable when go vet launches several analyzer processes.
func Emit(pass *analysis.Pass, event Event) {
	if !global.active.Load() || pass == nil {
		return
	}
	position := ""
	if event.Pos.IsValid() {
		position = pass.Fset.Position(event.Pos).String()
	}
	global.Lock()
	defer global.Unlock()
	if !selected(global.config, event, position) {
		return
	}
	encoded, err := json.Marshal(record{
		Analyzer: event.Analyzer,
		Check:    event.Check,
		Phase:    event.Phase,
		Reason:   event.Reason,
		Outcome:  event.Outcome,
		Position: position,
		Function: event.Function,
		Details:  event.Details,
	})
	if err != nil {
		return
	}
	encoded = append(encoded, '\n')
	_, _ = global.config.writer.Write(encoded)
}

func selected(config settings, event Event, position string) bool {
	if !config.selectors["all"] && !config.selectors[event.Analyzer] && (event.Check == "" || !config.selectors[event.Check]) {
		return false
	}
	if config.source != "" && !strings.Contains(position, config.source) {
		return false
	}
	return config.function == "" || strings.Contains(event.Function, config.function)
}
