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
		request := schedule.foreground.request
		status.State = stateChecking
		status.Text = "gohawk: checking…" + elapsedNote(request.progress)
		status.Tooltip = "Checking " + request.label + unsavedNote(request) + packagesNote(request.progress) +
			". The first check of a package also analyzes its dependencies once. Later checks reuse that work."
	case len(schedule.queue) > 0 || len(schedule.pending) > 0:
		status.State, status.Text = stateChecking, "gohawk: waiting for a pause"
	case schedule.skipped != "":
		status.State, status.Text, status.Tooltip = stateSkipped, "gohawk: not checked", schedule.skipped
	default:
		status = s.settledStatus(findings)
	}
	// The bar stays short: what is still running or was left out is one
	// plain phrase at most, and the details go in the tooltip.
	if schedule.background != nil && schedule.foreground == nil {
		status.Text += " · still checking…"
		subject := "the packages that use the one you edited"
		if schedule.background.request.kind == runWorkspace {
			subject = "every package in the module"
		}
		status.Tooltip = joinSentences(status.Tooltip, "Now checking "+subject+packagesNote(schedule.background.request.progress)+".")
	}
	if unchecked := s.uncheckedImporters(); unchecked > 0 && schedule.foreground == nil {
		status.Tooltip = joinSentences(status.Tooltip, fmt.Sprintf("%s the one you edited %s not re-checked, because there are more than"+
			" gohawk.importerLimit. Run “gohawk: Check Workspace” to check them all.", countUsers(unchecked), wasOrWere(unchecked)))
	}
	return status
}

func joinSentences(first, second string) string {
	return strings.TrimSpace(first + " " + second)
}

func countUsers(count int) string {
	if count == 1 {
		return "1 package that uses"
	}
	return fmt.Sprintf("%d packages that use", count)
}

func wasOrWere(count int) string {
	if count == 1 {
		return "was"
	}
	return "were"
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
		status.Text = unresolvedText(findings, "check of "+packageNames(stopped)+" stopped")
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
		status.Tooltip = "Last check: " + last.label + ", " + formatElapsed(last.elapsed) + "."
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

func (s *Server) uncheckedImporters() int {
	total := 0
	for _, count := range s.scheduler.unchecked {
		total += count
	}
	return total
}

// elapsedNote says how long a run has taken, once it has run long enough
// that the user may wonder. A quick check shows no note at all.
func elapsedNote(progress *runProgress) string {
	if progress == nil {
		return ""
	}
	elapsed := time.Since(progress.started)
	if elapsed < progressNoteAfter {
		return ""
	}
	return " " + formatDuration(elapsed)
}

// packagesNote says how many packages a long run has got through.
func packagesNote(progress *runProgress) string {
	if progress == nil {
		return ""
	}
	if packages := progress.packages.Load(); packages > 1 {
		return fmt.Sprintf(", %d packages so far", packages)
	}
	return ""
}

const progressNoteAfter = 3 * time.Second

func formatDuration(elapsed time.Duration) string {
	seconds := int(elapsed.Seconds())
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
}

func unsavedNote(request *runRequest) string {
	if len(request.texts) > 0 {
		return " (unsaved)"
	}
	return ""
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
