// Package trace provides opt-in structured evidence tracing for analyzers.
package trace

import (
	"encoding/json"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// event is one stable analyzer evidence decision. Details should contain only
// compact identifiers and counts; tracing is diagnostic metadata, not an SSA
// serialization format.
type event struct {
	Analyzer string
	Check    string
	Phase    Phase
	Reason   string
	Outcome  Outcome
	Pos      token.Pos
	// Candidate is the construct whose proof this event serves: the acquisition,
	// the spawn, the site a diagnostic would name. Much of a proof is evidence
	// about callee bodies in other files, or carries no position of its own, so
	// the candidate is what ties an event to the finding a reader is studying.
	Candidate token.Pos
	Function  string
	Details   map[string]string
}

// Record is one event as it is written: positions rendered as file:line:col.
type Record struct {
	Analyzer  string            `json:"analyzer"`
	Check     string            `json:"check,omitempty"`
	Phase     string            `json:"phase"`
	Reason    string            `json:"reason"`
	Outcome   Outcome           `json:"outcome"`
	Position  string            `json:"position,omitempty"`
	Candidate string            `json:"candidate,omitempty"`
	Function  string            `json:"function,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
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
	Phase      Phase
	Reason     string
	Outcome    Outcome
	Diagnostic analysis.Diagnostic
}

// EmitDiagnostic records a source-level analyzer decision using the same
// schema as deeper SSA evidence. Function names are resolved only while the
// corresponding trace selector is enabled.
func EmitDiagnostic(pass *analysis.Pass, diagnostic DiagnosticEvent) {
	if !Enabled(diagnostic.Analyzer, diagnostic.Diagnostic.Category) {
		return
	}
	details := map[string]string{}
	if diagnostic.Diagnostic.Message != "" {
		details["message"] = diagnostic.Diagnostic.Message
	}
	// A diagnostic names its own candidate, so the position a reader copies out
	// of a finding selects the report alongside the proof that produced it.
	write(pass, event{
		Analyzer:  diagnostic.Analyzer,
		Check:     diagnostic.Diagnostic.Category,
		Phase:     diagnostic.Phase,
		Reason:    diagnostic.Reason,
		Outcome:   diagnostic.Outcome,
		Pos:       diagnostic.Diagnostic.Pos,
		Candidate: diagnostic.Diagnostic.Pos,
		Function:  sourceFunction(pass, diagnostic.Diagnostic.Pos),
		Details:   details,
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

// write records one event as a JSON object when it matches the configured
// filters. Each event is marshaled and written in one call so append-mode trace files
// remain usable when go vet launches several analyzer processes.
func write(pass *analysis.Pass, entry event) {
	if !global.active.Load() || pass == nil {
		return
	}
	position := ""
	if entry.Pos.IsValid() {
		position = pass.Fset.Position(entry.Pos).String()
	}
	candidate := ""
	if entry.Candidate.IsValid() {
		candidate = pass.Fset.Position(entry.Candidate).String()
	}
	global.Lock()
	defer global.Unlock()
	if !selected(global.config, entry, candidate) {
		return
	}
	captured := Record{
		Analyzer:  entry.Analyzer,
		Check:     entry.Check,
		Phase:     entry.Phase.String(),
		Reason:    entry.Reason,
		Outcome:   entry.Outcome,
		Position:  position,
		Candidate: candidate,
		Function:  entry.Function,
		Details:   entry.Details,
	}
	if global.config.sink != nil {
		global.config.sink(captured)
		return
	}
	encoded, err := json.Marshal(captured)
	if err != nil {
		return
	}
	encoded = append(encoded, '\n')
	_, _ = global.config.writer.Write(encoded)
}

func selected(config settings, entry event, candidate string) bool {
	if !config.selectors["all"] && !config.selectors[entry.Analyzer] && (entry.Check == "" || !config.selectors[entry.Check]) {
		return false
	}
	// Selection keys on the candidate a proof serves rather than on the event's
	// own position. A proof spends most of its evidence on callee bodies in
	// other files, and many steps carry no position at all, so filtering by
	// event position discards the chain the reader asked to see.
	return config.candidate == "" || strings.Contains(candidate, config.candidate)
}

// enabledFor reports whether tracing selects this analyzer, check, and
// candidate. Analyzers call it once per candidate, so the evidence walk and the
// metadata it builds are skipped for candidates the selector excludes.
func enabledFor(pass *analysis.Pass, analyzer, check string, candidate token.Pos) bool {
	if !Enabled(analyzer, check) {
		return false
	}
	global.Lock()
	defer global.Unlock()
	if global.config.candidate == "" {
		return true
	}
	if pass == nil || !candidate.IsValid() {
		return false
	}
	return strings.Contains(pass.Fset.Position(candidate).String(), global.config.candidate)
}

// Step is the part of a trace event that varies between emissions. The
// analyzer, check, phase, and candidate come from the Probe, so no caller can
// emit a step that is not attributed to the proof it belongs to.
type Step struct {
	Reason   string
	Outcome  Outcome
	Pos      token.Pos
	Function string
	Details  map[string]string
}

// Probe emits the steps of one candidate's proof. Construct it once per
// candidate: it resolves selection up front, so a probe for a candidate the
// reader did not ask about is inert and its caller can skip building metadata.
type Probe struct {
	pass      *analysis.Pass
	analyzer  string
	check     string
	candidate token.Pos
	enabled   bool
}

// For binds tracing to the candidate whose proof the caller is about to build.
func For(pass *analysis.Pass, analyzer, check string, candidate token.Pos) Probe {
	return Probe{
		pass: pass, analyzer: analyzer, check: check, candidate: candidate,
		enabled: enabledFor(pass, analyzer, check, candidate),
	}
}

// ForPackage binds tracing to work that belongs to no single candidate, such as
// a decision to skip a package. It is the deliberate exception to attribution,
// and a candidate selector never matches it.
func ForPackage(pass *analysis.Pass, analyzer, check string) Probe {
	return Probe{pass: pass, analyzer: analyzer, check: check, enabled: Enabled(analyzer, check)}
}

// Enabled reports whether this probe emits, so a caller can skip metadata that
// is only worth building for a traced candidate.
func (probe Probe) Enabled() bool { return probe.enabled }

// Evidence records a fact that supports or rejects the candidate.
func (probe Probe) Evidence(step Step) { probe.emit(PhaseEvidence, step) }

// Decision records the outcome the proof reached for the candidate.
func (probe Probe) Decision(step Step) { probe.emit(PhaseDecision, step) }

// Label records the label a lifecycle classifier gave one instruction on the
// candidate's path, and why, so a reader sees what the flow query was asked
// to combine without reading the evidence behind each label.
func (probe Probe) Label(step Step) { probe.emit(PhaseLabel, step) }

// Candidate records that a potentially reportable construct was observed.
func (probe Probe) Candidate(step Step) { probe.emit(PhaseCandidate, step) }

// Considered records a proof step that was evaluated and did not hold, so a
// reader can see which suppressions were tried before the reported reason won.
func (probe Probe) Considered(step Step) { probe.emit(PhaseConsidered, step) }

// Observe records a give-up reported by a shared proof: the reason the proof
// stopped, where, and what blocked it. Its signature uses only primitives so
// ssaflow's Observer can be satisfied by this method value without importing
// the tracer.
func (probe Probe) Observe(reason string, at token.Pos, details map[string]string) {
	probe.emit(PhaseEvidence, Step{Reason: reason, Outcome: OutcomeUnknown, Pos: at, Details: details})
}

// Observer returns Observe as a plain function when the probe is enabled and
// nil otherwise, so a budget attached to a disabled probe stays silent and
// builds no details.
func (probe Probe) Observer() func(reason string, at token.Pos, details map[string]string) {
	if !probe.enabled {
		return nil
	}
	return probe.Observe
}

func (probe Probe) emit(phase Phase, step Step) {
	if !probe.enabled {
		return
	}
	write(probe.pass, event{
		Analyzer:  probe.analyzer,
		Check:     probe.check,
		Phase:     phase,
		Reason:    step.Reason,
		Outcome:   step.Outcome,
		Pos:       step.Pos,
		Candidate: probe.candidate,
		Function:  step.Function,
		Details:   step.Details,
	})
}
