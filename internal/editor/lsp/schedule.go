package lsp

import (
	"context"
	"fmt"
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
	// isolated marks queued directories that are re-checked on their own after
	// a multi-package run failed. Their imports did not change, so they do not
	// start another importer run.
	isolated   map[string]bool
	ripple     map[string]bool // checked directories whose importers are due
	workspaces map[string]bool // module roots due a whole-module run
	// unchecked counts, per module, the importers the last importer run
	// left out because there were more than the limit. Their findings may be
	// out of date until a whole-module run.
	unchecked  map[string]int
	beating    bool // a status heartbeat is scheduled
	foreground *activeRun
	background *activeRun
	last       runSummary
	skipped    string
	nextRunID  int
}

func newScheduler() scheduler {
	return scheduler{
		pending: make(map[string]bool), isolated: make(map[string]bool),
		ripple: make(map[string]bool), workspaces: make(map[string]bool), unchecked: make(map[string]int),
	}
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
	s.requestDirectory(ctx, filepath.Dir(path), delay)
}

// requestDirectory asks for one package directory to be checked after delay.
func (s *Server) requestDirectory(ctx context.Context, directory string, delay time.Duration) {
	s.cancelRunsFor(directory)
	delete(s.scheduler.isolated, directory)
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
		s.enqueue(directory)
	}
	s.pump(ctx)
}

func (s *Server) enqueue(directory string) {
	if !slices.Contains(s.scheduler.queue, directory) {
		s.scheduler.queue = append(s.scheduler.queue, directory)
	}
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
		root, ok := moduleRoot(directory)
		if !ok {
			delete(schedule.isolated, directory)
			s.logf("skipped   %s: no go.mod above it", directory)
			s.pump(ctx)
			return
		}
		request := &runRequest{kind: runPackage, root: root, directories: []string{directory}, label: filepath.Base(directory)}
		request.startsRipple = !schedule.isolated[directory]
		delete(schedule.isolated, directory)
		schedule.foreground = s.start(ctx, request)
		return
	}
	if schedule.background != nil {
		return
	}
	if root, directories, ok := s.nextRipple(); ok {
		schedule.background = s.start(ctx, &runRequest{kind: runImporters, root: root, directories: directories, label: "importers"})
		return
	}
	if roots := sortedKeys(schedule.workspaces); len(roots) > 0 {
		root := roots[0]
		delete(schedule.workspaces, root)
		// A whole-module run also answers for packages that no longer exist,
		// so the directories already stored for the module are covered too.
		request := &runRequest{kind: runWorkspace, root: root, directories: s.results.directoriesOfModule(root), label: "workspace"}
		schedule.background = s.start(ctx, request)
	}
}

// nextRipple takes the changed directories of one module off the ripple set.
func (s *Server) nextRipple() (string, []string, bool) {
	if s.settings.Scope != scopeThenImporters {
		s.scheduler.ripple = make(map[string]bool)
		return "", nil, false
	}
	// A directory whose module is gone, as when go.mod was deleted during
	// its check, has no importers to find. Leaving it in the set would start
	// an empty run again and again.
	for directory := range s.scheduler.ripple {
		if _, ok := moduleRoot(directory); !ok {
			delete(s.scheduler.ripple, directory)
		}
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

func (s *Server) start(ctx context.Context, request *runRequest) *activeRun {
	s.scheduler.nextRunID++
	request.id = s.scheduler.nextRunID
	// Unsaved buffers are analyzed only when checks follow typing. In the
	// other modes findings describe the files as last saved.
	if s.settings.Run == runOnPause {
		request.texts = s.documents.unsavedTexts(s.insideWorkspace)
	}
	request.progress = newRunProgress()
	if request.kind == runImporters {
		request.openDirectories = s.openDirectories()
	}
	limits := runLimits{
		idle:          s.settings.timeout(),
		memoryBytes:   s.settings.memoryBytes(),
		memoryLimit:   s.settings.MemoryLimit,
		parallel:      s.settings.MaxParallel,
		experimental:  s.settings.Experimental,
		importerLimit: s.settings.importerLimit(),
	}
	s.heartbeat()
	s.logf("check     %s%s", request.label, strings.TrimSuffix(unsavedNote(request), "…"))
	s.eventLog.record(event{
		Event: "run-started", Run: request.id, Detail: request.kind.String(), Label: request.label, Unsaved: len(request.texts) > 0,
	})
	runContext, cancel := context.WithCancel(ctx)
	s.runs.Go(func() {
		result := s.runner.execute(runContext, request, limits)
		// Posting must not hold the run open: a stopping server waits for
		// runs to end and no longer reads what they post.
		go s.post(func() { s.finish(ctx, result) })
	})
	return &activeRun{request: request, cancel: cancel}
}

// openDirectories lists the packages that have a checkable file open.
func (s *Server) openDirectories() []string {
	directories := make(map[string]bool)
	for path := range s.documents.byPath {
		if s.skipReason(path) == "" {
			directories[filepath.Dir(path)] = true
		}
	}
	return sortedKeys(directories)
}

// heartbeat refreshes the status every few seconds while a run is active, so
// a long check shows how far it has got instead of looking stuck.
func (s *Server) heartbeat() {
	if s.scheduler.beating {
		return
	}
	s.scheduler.beating = true
	time.AfterFunc(heartbeatInterval, func() {
		s.post(func() {
			s.scheduler.beating = false
			if s.scheduler.foreground != nil || s.scheduler.background != nil {
				s.publishStatus()
				s.heartbeat()
			}
		})
	})
}

const heartbeatInterval = 2 * time.Second

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
	if s.stopExplain != nil {
		s.stopExplain()
	}
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
	s.recordFinished(result)
	s.apply(result)
	// The status names the last package the user edited. A clean background
	// run is not news; a background run that failed or was stopped is.
	if result.request.kind == runPackage || result.outcome != outcomeCompleted {
		schedule.last = runSummary{label: result.request.label, outcome: result.outcome, elapsed: result.elapsed, valid: true}
	}
	s.pump(ctx)
}

func (s *Server) recordFinished(result runResult) {
	if !s.eventLog.enabled() {
		return
	}
	findings := len(result.diagnostics)
	serverBytes, goroutines := serverFootprint()
	s.eventLog.record(event{
		Event: "run-finished", Run: result.request.id, Detail: result.request.kind.String(), Label: result.request.label,
		Outcome: result.outcome.String(), ElapsedMS: result.elapsed.Milliseconds(), PeakBytes: result.peakBytes, Findings: &findings,
		ServerBytes: serverBytes, Goroutines: goroutines,
	})
}

func (s *Server) apply(result runResult) {
	request := result.request
	if result.commandLine != "" {
		s.logf("ran       %s", result.commandLine)
	}
	switch result.outcome {
	case outcomeCompleted:
		s.applyCompleted(result)
	case outcomeBuildFailed:
		s.applyBuildFailure(result)
	case outcomeTimedOut:
		s.results.markUnresolved(result.covered, result.outcome)
		s.logf("stopped   %s after %s (time limit)", request.label, formatElapsed(result.elapsed))
	case outcomeMemoryExceeded:
		s.results.markUnresolved(result.covered, result.outcome)
		s.logf("stopped   %s at %s (memory limit %s)", request.label, formatBytes(result.peakBytes), s.settings.MemoryLimit)
	case outcomeFailed:
		s.logf("error     %s: %v", request.label, result.err)
	case outcomeCanceled:
	}
}

func (s *Server) applyCompleted(result runResult) {
	request := result.request
	fresh := newConverter(s.options.Catalog, result.overlay, s.spelling).convert(result.diagnostics)
	touched, refused := s.results.replace(result.covered, fresh)
	s.publish(touched)
	for _, finding := range refused {
		// Never silent: a finding that cannot be filed is a finding the user
		// does not see.
		s.logf("dropped   finding at %s, outside the packages this run checked: %s", finding.path, finding.diagnostic.Message)
	}
	for _, analysisError := range result.analysisErrors {
		s.logf("error     %s", analysisError)
	}
	if request.startsRipple {
		s.scheduler.ripple[request.directories[0]] = true
	}
	switch request.kind {
	case runImporters:
		if result.skippedImporters > 0 {
			s.scheduler.unchecked[request.root] = result.skippedImporters
			s.logf("skipped   %d importers of %s: more than the limit of %d, and not open", result.skippedImporters,
				strings.Join(relativePatterns(request.root, request.directories), " "), s.settings.importerLimit())
		} else {
			delete(s.scheduler.unchecked, request.root)
		}
	case runWorkspace:
		delete(s.scheduler.unchecked, request.root)
	case runPackage:
	}
	s.logf("done      %s  %s%s  %s", request.label, formatElapsed(result.elapsed), formatPeak(result.peakBytes), countFindings(len(fresh)))
}

// applyBuildFailure records what a failed build leaves unknown. One package
// that does not compile is simply unresolved. When a run over several
// packages fails, the output does not say which ones were analyzed, so each
// is checked again on its own: the ones that build get fresh findings and the
// ones that do not are marked unresolved individually.
func (s *Server) applyBuildFailure(result runResult) {
	request := result.request
	s.logf("failed    %s: %s", request.label, firstLines(result.stderr, 3))
	if request.kind == runPackage {
		s.results.markUnresolved(result.covered, outcomeBuildFailed)
		return
	}
	for _, directory := range result.covered {
		if !slices.Contains(s.scheduler.queue, directory) {
			s.scheduler.isolated[directory] = true
			s.enqueue(directory)
		}
	}
}

// maxFindingsPerFile bounds one published list. A generated file can carry
// thousands of findings; sending them all makes one message of many
// megabytes that no reader will go through.
const maxFindingsPerFile = 200

func (s *Server) publish(paths []string) {
	for _, path := range paths {
		findings := s.results.forFile(path)
		count := len(findings)
		if count > maxFindingsPerFile {
			// Never silent: the list says it was cut and how long it really
			// is. The store keeps every finding, so the status count is whole.
			findings = append(findings[:maxFindingsPerFile:maxFindingsPerFile], diagnostic{
				Severity: severityInformation,
				Source:   diagnosticSource,
				Message: fmt.Sprintf("gohawk has %d findings in this file and shows the first %d. Run gohawk on the command line to see them all.",
					count, maxFindingsPerFile),
			})
			s.logf("capped    %s: %d findings, %d shown", path, count, maxFindingsPerFile)
		}
		params := publishDiagnosticsParams{URI: uriFromPath(path), Diagnostics: findings}
		s.eventLog.record(event{Event: "published", Path: path, Findings: &count})
		if err := s.connection.notify(methodPublishDiagnostics, params); err != nil {
			return
		}
	}
}

func firstLines(text string, limit int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.Join(lines[:min(limit, len(lines))], " | ")
}
