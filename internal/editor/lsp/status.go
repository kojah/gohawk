package lsp

import (
	"fmt"
	"path/filepath"
	"strings"
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

// settledStatus describes the state when nothing is running in front. It is
// derived from what is stored, not from the last run alone: a package whose
// findings are unknown stays named until a check of it completes, however
// many other packages are checked in between.
func (s *Server) settledStatus(findings int) StatusParams {
	last := s.scheduler.last
	status := StatusParams{Findings: findings, State: stateIdle, Text: "gohawk: no findings"}
	if findings > 0 {
		status.State, status.Text = stateFindings, "gohawk: "+countFindings(findings)
	}
	stopped, _ := s.results.unresolvedBy(outcomeTimedOut, outcomeMemoryExceeded)
	broken, staleFindings := s.results.unresolvedBy(outcomeBuildFailed)
	switch {
	case len(stopped) > 0:
		status.State = stateStopped
		status.Text = unresolvedText(findings, "stopped at a limit: "+packageNames(stopped))
		status.Tooltip = "A check passed its time or memory limit and was stopped. The findings of that package are unknown."
	case len(broken) > 0:
		status.State = stateStale
		status.Text = unresolvedText(findings, "build failed in "+packageNames(broken))
		status.Tooltip = "The package does not compile, so it cannot be checked."
		if staleFindings > 0 {
			status.Tooltip += " Its " + countFindings(staleFindings) + " from before may be stale."
		}
	case last.valid && last.outcome == outcomeFailed:
		status.State, status.Text = stateError, "gohawk: check failed, see Output"
	case !last.valid:
		if findings == 0 {
			status.Text = "gohawk"
		}
	case last.outcome == outcomeCompleted:
		status.Text += " · " + last.label + " " + formatElapsed(last.elapsed)
	}
	return status
}

// unresolvedText leads with the findings that are known and then names what
// is not, so the count is never read as the whole truth.
func unresolvedText(findings int, problem string) string {
	if findings > 0 {
		return "gohawk: " + countFindings(findings) + " · " + problem
	}
	return "gohawk: " + problem
}

func packageNames(directories []string) string {
	if len(directories) == 1 {
		return filepath.Base(directories[0])
	}
	return fmt.Sprintf("%d packages", len(directories))
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

// shellJoin renders arguments for the log. It is for reading, not for
// execution: the command itself never passes through a shell.
func shellJoin(arguments []string) string {
	quoted := make([]string, len(arguments))
	for index, argument := range arguments {
		if strings.ContainsAny(argument, " \t\"'") {
			argument = "'" + strings.ReplaceAll(argument, "'", `'\''`) + "'"
		}
		quoted[index] = argument
	}
	return strings.Join(quoted, " ")
}
