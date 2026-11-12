package lsp

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Scheduling policy. The edited package is always checked first and alone,
// because that is fast. Packages that import it are checked afterwards in one
// background run, because facts flow along imports and their findings may
// have changed; that run is the expensive one, so any new edit cancels it.
// At most one foreground and one background run exist at a time.

type activeRun struct {
	request *runRequest
	cancel  context.CancelFunc
}

type runSummary struct {
	label   string
	outcome runOutcome
	elapsed time.Duration
	valid   bool
}

type scheduler struct {
	pending    map[string]bool // package directories waiting for the pause delay
	generation int             // invalidates timers superseded by a newer edit
	queue      []string
	ripple     map[string]bool // checked directories whose importers are due
	workspaces map[string]bool // module roots due a whole-module run
	foreground *activeRun
	background *activeRun
	last       runSummary
	skipped    string
	nextRunID  int
}

func newScheduler() scheduler {
	return scheduler{pending: make(map[string]bool), ripple: make(map[string]bool), workspaces: make(map[string]bool)}
}

// requestCheck asks for the package holding path to be checked after delay.
func (s *Server) requestCheck(ctx context.Context, path string, delay time.Duration) {
	if reason := s.skipReason(path); reason != "" {
		// A file opened in the background must not replace the status of
		// the code the user is working on.
		if s.insideWorkspace(path) {
			s.scheduler.skipped = reason
			s.publishStatus()
		}
		return
	}
	s.scheduler.skipped = ""
	directory := filepath.Dir(path)
	s.cancelRunsFor(directory)
	s.scheduler.pending[directory] = true
	s.scheduler.generation++
	if delay <= 0 {
		s.flush(ctx)
		return
	}
	generation := s.scheduler.generation
	time.AfterFunc(delay, func() {
		s.post(func() {
			if generation == s.scheduler.generation {
				s.flush(ctx)
			}
		})
	})
	s.publishStatus()
}

func (s *Server) flush(ctx context.Context) {
	directories := sortedKeys(s.scheduler.pending)
	s.scheduler.pending = make(map[string]bool)
	for _, directory := range directories {
		if !slices.Contains(s.scheduler.queue, directory) {
			s.scheduler.queue = append(s.scheduler.queue, directory)
		}
	}
	s.pump(ctx)
}

// pump starts the next run the policy allows and refreshes the status.
func (s *Server) pump(ctx context.Context) {
	defer s.publishStatus()
	if s.shutdown {
		return
	}
	schedule := &s.scheduler
	if schedule.foreground != nil {
		return
	}
	if len(schedule.queue) > 0 {
		directory := schedule.queue[0]
		schedule.queue = schedule.queue[1:]
		root, _ := moduleRoot(directory)
		schedule.foreground = s.start(ctx, runPackage, root, []string{directory}, filepath.Base(directory))
		return
	}
	if schedule.background != nil {
		return
	}
	if root, directories, ok := s.nextRipple(); ok {
		schedule.background = s.start(ctx, runImporters, root, directories, "importers")
		return
	}
	if roots := sortedKeys(schedule.workspaces); len(roots) > 0 {
		delete(schedule.workspaces, roots[0])
		schedule.background = s.start(ctx, runWorkspace, roots[0], nil, "workspace")
	}
}

// nextRipple takes the changed directories of one module off the ripple set.
func (s *Server) nextRipple() (string, []string, bool) {
	if s.settings.Scope != scopeThenImporters {
		s.scheduler.ripple = make(map[string]bool)
		return "", nil, false
	}
	changed := sortedKeys(s.scheduler.ripple)
	if len(changed) == 0 {
		return "", nil, false
	}
	root, _ := moduleRoot(changed[0])
	var directories []string
	for _, directory := range changed {
		if insideDirectory(root, directory) {
			directories = append(directories, directory)
			delete(s.scheduler.ripple, directory)
		}
	}
	return root, directories, true
}

func (s *Server) start(ctx context.Context, kind runKind, root string, directories []string, label string) *activeRun {
	s.scheduler.nextRunID++
	request := &runRequest{
		id: s.scheduler.nextRunID, kind: kind, root: root, directories: directories,
		texts: s.documents.unsavedTexts(), label: label,
	}
	limits := runLimits{
		timeout:      s.settings.timeout(),
		memoryBytes:  s.settings.memoryBytes(),
		memoryLimit:  s.settings.MemoryLimit,
		parallel:     s.settings.MaxParallel,
		experimental: s.settings.Experimental,
	}
	s.logf("check     %s%s", label, strings.TrimSuffix(unsavedNote(request), "…"))
	s.eventLog.record(event{Event: "run-started", Run: request.id, Detail: kind.String(), Label: label, Unsaved: len(request.texts) > 0})
	runContext, cancel := context.WithCancel(ctx)
	go func() {
		result := s.runner.execute(runContext, request, limits)
		s.post(func() { s.finish(ctx, result) })
	}()
	return &activeRun{request: request, cancel: cancel}
}

// cancelRunsFor stops work an edit in directory has made pointless: a
// foreground check of that package, and any background run, whose inputs
// include the edited package's facts.
func (s *Server) cancelRunsFor(directory string) {
	schedule := &s.scheduler
	if active := schedule.foreground; active != nil && slices.Contains(active.request.directories, directory) {
		active.cancel()
		schedule.foreground = nil
	}
	if active := schedule.background; active != nil {
		active.cancel()
		schedule.background = nil
		// The work is still owed once the user pauses again.
		switch active.request.kind {
		case runImporters:
			for _, changed := range active.request.directories {
				schedule.ripple[changed] = true
			}
		case runWorkspace:
			schedule.workspaces[active.request.root] = true
		case runPackage:
		}
	}
}

func (s *Server) cancelAllRuns() {
	for _, active := range []*activeRun{s.scheduler.foreground, s.scheduler.background} {
		if active != nil {
			active.cancel()
		}
	}
	s.scheduler.foreground, s.scheduler.background = nil, nil
}

// finish applies a completed run. A run that is no longer the active one was
// cancelled by a newer edit, so its result describes old text and is dropped.
func (s *Server) finish(ctx context.Context, result runResult) {
	defer result.overlay.remove()
	schedule := &s.scheduler
	switch {
	case schedule.foreground != nil && schedule.foreground.request == result.request:
		schedule.foreground.cancel()
		schedule.foreground = nil
	case schedule.background != nil && schedule.background.request == result.request:
		schedule.background.cancel()
		schedule.background = nil
	default:
		return
	}
	findings := len(result.diagnostics)
	s.eventLog.record(event{
		Event: "run-finished", Run: result.request.id, Detail: result.request.kind.String(), Label: result.request.label,
		Outcome: result.outcome.String(), ElapsedMS: result.elapsed.Milliseconds(), PeakBytes: result.peakBytes, Findings: &findings,
	})
	s.apply(result)
	// The status names the last package the user edited. A clean background
	// run is not news; a background run that failed or was stopped is.
	if result.request.kind == runPackage || result.outcome != outcomeCompleted {
		schedule.last = runSummary{label: result.request.label, outcome: result.outcome, elapsed: result.elapsed, valid: true}
	}
	s.pump(ctx)
}

func (s *Server) apply(result runResult) {
	request := result.request
	if result.commandLine != "" {
		s.logf("ran       %s", result.commandLine)
	}
	switch result.outcome {
	case outcomeCompleted:
		fresh := newConverter(s.options.Catalog, result.overlay).convert(result.diagnostics)
		covered := result.covered
		if request.kind == runWorkspace {
			covered = s.results.directoriesUnder(request.root, fresh)
		}
		s.publish(s.results.replace(covered, fresh))
		for _, analysisError := range result.analysisErrors {
			s.logf("error     %s", analysisError)
		}
		if request.kind == runPackage {
			s.scheduler.ripple[request.directories[0]] = true
		}
		s.logf("done      %s  %s%s  %s", request.label, formatElapsed(result.elapsed), formatPeak(result.peakBytes), countFindings(len(fresh)))
	case outcomeBuildFailed:
		if request.kind == runPackage {
			s.results.markStale(request.directories)
		}
		s.logf("failed    %s: %s", request.label, firstLines(result.stderr, 3))
	case outcomeTimedOut:
		s.logf("stopped   %s after %s (time limit)", request.label, formatElapsed(result.elapsed))
	case outcomeMemoryExceeded:
		s.logf("stopped   %s at %s (memory limit %s)", request.label, formatBytes(result.peakBytes), s.settings.MemoryLimit)
	case outcomeFailed:
		s.logf("error     %s: %v", request.label, result.err)
	case outcomeCanceled:
	}
}

func (s *Server) publish(paths []string) {
	for _, path := range paths {
		params := publishDiagnosticsParams{URI: uriFromPath(path), Diagnostics: s.results.forFile(path)}
		count := len(params.Diagnostics)
		s.eventLog.record(event{Event: "published", Path: path, Findings: &count})
		if err := s.connection.notify(methodPublishDiagnostics, params); err != nil {
			return
		}
	}
}

func firstLines(text string, limit int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	kept := lines[:min(limit, len(lines))]
	return strings.Join(kept, " | ")
}
