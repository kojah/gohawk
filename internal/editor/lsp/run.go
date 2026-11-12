package lsp

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/kojah/gohawk/internal/reporting/vetjson"
)

// One check is one `go vet` child process with gohawk as its tool. All
// analysis memory lives in that process tree and is released when it exits;
// the server keeps only the decoded findings. A run that passes its time or
// memory limit is stopped and reported as stopped, never as a clean result.

type runKind int

const (
	runPackage   runKind = iota // the edited package alone
	runImporters                // packages that import recently checked ones
	runWorkspace                // every package of one module
)

type runOutcome int

const (
	outcomeCompleted   runOutcome = iota // go vet ran; findings are current
	outcomeBuildFailed                   // the packages did not compile
	outcomeCanceled                      // a newer edit made the run pointless
	outcomeTimedOut
	outcomeMemoryExceeded
	outcomeFailed // the run could not be carried out
)

type runRequest struct {
	id   int
	kind runKind
	// root is the module root. directories are the packages a package run
	// checks, or the changed packages whose importers an importer run finds.
	root        string
	directories []string
	texts       map[string]string // unsaved buffers at the time of the request
	label       string
	// startsRipple says a completed package run should be followed by a
	// check of the packages that import it.
	startsRipple bool
	// openDirectories are the packages with a file open in the editor. A
	// widely imported package is followed only into these.
	openDirectories []string
	progress        *runProgress
}

type runResult struct {
	request        *runRequest
	outcome        runOutcome
	diagnostics    []vetjson.Diagnostic
	analysisErrors []string
	// covered lists the package directories whose findings this result
	// replaces. A workspace run replaces everything under its root.
	covered []string
	// skippedImporters counts importers left unchecked because the changed
	// package has more of them than the limit.
	skippedImporters int
	overlay          *overlay
	stderr           string
	err              error
	elapsed          time.Duration
	peakBytes        int64 // zero when the platform cannot measure it
	commandLine      string
}

type runner struct {
	executable string
	temporary  string
}

type runLimits struct {
	// idle is how long a run may go without the go command starting or
	// finishing a package. It bounds one package's analysis, not the run: a
	// first check that builds facts for thousands of dependencies keeps
	// making progress and is allowed to finish.
	idle         time.Duration
	memoryBytes  int64
	memoryLimit  string
	parallel     int
	experimental bool
	// importerLimit is the most importers a background run follows; beyond
	// it only importers with an open file are checked. Zero means no limit.
	importerLimit int
}

// longestRun bounds a whole run however steadily it progresses.
const longestRun = 30 * time.Minute

// withinImporterLimit returns the importers to check. A widely imported
// package can have thousands, and re-checking them all after every pause
// would keep the machine busy for minutes each time. Past the limit only the
// importers the user has open are followed; the rest are counted, so the
// status can say they were not re-checked.
func withinImporterLimit(importers, open []string, limit int) (kept []string, skipped int) {
	if limit <= 0 || len(importers) <= limit {
		return importers, 0
	}
	for _, directory := range importers {
		if slices.Contains(open, directory) {
			kept = append(kept, directory)
		}
	}
	return kept, len(importers) - len(kept)
}

func (r *runner) execute(ctx context.Context, request *runRequest, limits runLimits) (result runResult) {
	result = runResult{request: request}
	started := time.Now()
	defer func() { result.elapsed = time.Since(started) }()

	// One set of limits covers everything the run starts: the package
	// listing as well as the analysis.
	runContext, cancel := context.WithTimeout(ctx, longestRun)
	defer cancel()
	guarded := newGuard(limits, cancel, request.progress)

	var err error
	result.overlay, err = writeOverlay(runContext, r.temporary, request.texts)
	if err != nil {
		result.outcome, result.err = interrupted(ctx, runContext, guarded, outcomeFailed), err
		return result
	}
	overlayFile := ""
	if result.overlay != nil {
		overlayFile = result.overlay.file
	}
	directory, patterns, err := r.target(runContext, guarded, request, overlayFile, &result)
	result.peakBytes = guarded.peak.Load()
	if err != nil {
		result.outcome, result.err = interrupted(ctx, runContext, guarded, outcomeFailed), err
		return result
	}
	// -v makes the go command name each package as it works on it, which is
	// the run's progress signal.
	arguments := []string{"vet", "-v", "-vettool=" + r.executable, "-json", "-p=" + strconv.Itoa(limits.parallel)}
	if overlayFile != "" {
		arguments = append(arguments, "-overlay="+overlayFile)
	}
	if limits.experimental {
		arguments = append(arguments, "-tier=experimental")
	}
	// A wide module can import a package from more directories than one
	// command line holds, so the patterns are checked in batches.
	for batch := range slices.Chunk(patterns, patternsPerCommand) {
		command := append(slices.Clone(arguments), batch...)
		if result.commandLine == "" {
			result.commandLine = "go " + shellJoin(command)
		}
		output, waitErr := guarded.run(runContext, directory, command)
		result.peakBytes = guarded.peak.Load()
		result.stderr += string(output.stderr)
		if stopped := interrupted(ctx, runContext, guarded, outcomeCompleted); stopped != outcomeCompleted {
			result.outcome = stopped
			return result
		}
		if output.truncated {
			result.outcome, result.err = outcomeFailed, errOutputTooLarge
			return result
		}
		decodeVetOutput(output.stdout, waitErr, &result)
		if result.outcome != outcomeCompleted {
			return result
		}
	}
	return result
}

// patternsPerCommand bounds one go vet command line.
const patternsPerCommand = 256

var errOutputTooLarge = errors.New("go vet printed more output than the server accepts")

// target resolves what a request checks: the working directory, the package
// patterns, and the directories whose findings the result will replace.
func (r *runner) target(ctx context.Context, guarded *guard, request *runRequest, overlayFile string, result *runResult) (string, []string, error) {
	list := func(patterns []string) ([]listedPackage, error) {
		return listPackages(ctx, guarded, request.root, overlayFile, patterns)
	}
	switch request.kind {
	case runPackage:
		result.covered = request.directories
		return request.directories[0], []string{"."}, nil
	case runWorkspace:
		// The run answers for every package the module lists, and for the
		// directories it already had findings for, which may be gone now.
		listed, err := list([]string{"./..."})
		if err != nil {
			return "", nil, err
		}
		covered := make(map[string]bool, len(listed)+len(request.directories))
		for _, listedPackage := range listed {
			covered[filepath.Clean(listedPackage.Dir)] = true
		}
		for _, directory := range request.directories {
			covered[directory] = true
		}
		result.covered = sortedKeys(covered)
		return request.root, []string{"./..."}, nil
	default:
		directories, err := importers(ctx, guarded, request.root, overlayFile, request.directories)
		if err != nil {
			return "", nil, err
		}
		directories, result.skippedImporters = withinImporterLimit(directories, request.openDirectories, guarded.limits.importerLimit)
		result.covered = directories
		return request.root, relativePatterns(request.root, directories), nil
	}
}

// decodeVetOutput classifies a finished go vet. go vet prints analysis results
// as JSON on stdout and build failures on stderr, so a failing exit with no
// JSON means the packages did not compile and earlier findings stay as they
// were. In JSON mode findings are data and do not fail the process.
func decodeVetOutput(stdout []byte, waitErr error, result *runResult) {
	merged, mergeErr := vetjson.Merge(stdout)
	if mergeErr != nil || waitErr != nil && len(bytes.TrimSpace(stdout)) == 0 {
		result.outcome, result.err = outcomeBuildFailed, waitErr
		return
	}
	diagnostics, analysisErrors, err := vetjson.Decode(merged)
	if err != nil {
		result.outcome, result.err = outcomeFailed, err
		return
	}
	result.diagnostics = append(result.diagnostics, diagnostics...)
	result.analysisErrors = append(result.analysisErrors, analysisErrors...)
	if waitErr != nil {
		// Some packages were analyzed and others failed to build. Which ones
		// failed is not recoverable from the output, so nothing is replaced.
		result.outcome, result.err = outcomeBuildFailed, waitErr
	}
}
