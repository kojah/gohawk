package lsp

import (
	"fmt"
	"time"
)

// The status line every client shows. The server composes the wording so
// editors agree on what each state means; a client only picks an icon from
// the state. A stopped or failed run is always named as such, so the absence
// of findings is never mistaken for a clean result.

const (
	stateIdle     = "idle"
	stateFindings = "findings"
	stateChecking = "checking"
	stateStale    = "stale"
	stateStopped  = "stopped"
	stateError    = "error"
	stateSkipped  = "skipped"
)

func (s *Server) publishStatus() {
	status := s.status()
	if status == s.lastStatus {
		return
	}
	s.lastStatus = status
	s.eventLog.record(event{Event: "status", State: status.State, Text: status.Text})
	_ = s.connection.notify(methodStatus, status)
}

func (s *Server) status() StatusParams {
	schedule := &s.scheduler
	findings := s.results.total()
	status := StatusParams{Findings: findings}
	switch {
	case schedule.foreground != nil:
		status.State = stateChecking
		status.Text = "gohawk: checking " + schedule.foreground.request.label + unsavedNote(schedule.foreground.request)
	case len(schedule.queue) > 0 || len(schedule.pending) > 0:
		status.State, status.Text = stateChecking, "gohawk: waiting for a pause"
	case schedule.skipped != "":
		status.State, status.Text, status.Tooltip = stateSkipped, "gohawk: not checked", schedule.skipped
	default:
		status = s.settledStatus(findings)
	}
	if schedule.background != nil && schedule.foreground == nil {
		status.Text += " · checking " + schedule.background.request.label
	}
	return status
}

// settledStatus describes the state when nothing is running in front.
func (s *Server) settledStatus(findings int) StatusParams {
	last := s.scheduler.last
	status := StatusParams{Findings: findings, State: stateIdle, Text: "gohawk: no findings"}
	if findings > 0 {
		status.State, status.Text = stateFindings, "gohawk: "+countFindings(findings)
	}
	if !last.valid {
		status.Text = "gohawk"
		return status
	}
	switch last.outcome {
	case outcomeTimedOut:
		status.State = stateStopped
		status.Text = fmt.Sprintf("gohawk: %s stopped after %s", last.label, formatElapsed(last.elapsed))
		status.Tooltip = "The check passed its time limit and was stopped. Its findings are unknown."
	case outcomeMemoryExceeded:
		status.State = stateStopped
		status.Text = fmt.Sprintf("gohawk: %s stopped at %s", last.label, s.settings.MemoryLimit)
		status.Tooltip = "The check passed its memory limit and was stopped. Its findings are unknown."
	case outcomeBuildFailed:
		status.State, status.Text = stateStale, "gohawk: build failed"
		if s.results.anyStale() {
			status.Text += ", " + countFindings(findings) + " may be stale"
		}
		status.Tooltip = "The package does not compile. Earlier findings are kept until it does."
	case outcomeFailed:
		status.State, status.Text = stateError, "gohawk: check failed, see Output"
	case outcomeCompleted:
		status.Text += " · " + last.label + " " + formatElapsed(last.elapsed)
	case outcomeCanceled:
	}
	return status
}

func unsavedNote(request *runRequest) string {
	if len(request.texts) > 0 {
		return " (unsaved)…"
	}
	return "…"
}

func countFindings(count int) string {
	if count == 1 {
		return "1 finding"
	}
	return fmt.Sprintf("%d findings", count)
}

func formatElapsed(elapsed time.Duration) string {
	return fmt.Sprintf("%.1fs", elapsed.Seconds())
}

func formatPeak(bytes int64) string {
	if bytes <= 0 {
		return ""
	}
	return "  peak " + formatBytes(bytes)
}

func formatBytes(bytes int64) string {
	const mebibyte = 1 << 20
	if bytes >= 1<<30 {
		return fmt.Sprintf("%.1f GiB", float64(bytes)/(1<<30))
	}
	return fmt.Sprintf("%d MiB", bytes/mebibyte)
}
