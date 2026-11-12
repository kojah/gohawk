package lsp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
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
}

type runResult struct {
	request        *runRequest
	outcome        runOutcome
	diagnostics    []vetjson.Diagnostic
	analysisErrors []string
	// covered lists the package directories whose findings this result
	// replaces. A workspace run replaces everything under its root.
	covered     []string
	overlay     *overlay
	stderr      string
	err         error
	elapsed     time.Duration
	peakBytes   int64 // zero when the platform cannot measure it
	commandLine string
}

type runner struct {
	executable string
	temporary  string
}

type runLimits struct {
	timeout      time.Duration
	memoryBytes  int64
	memoryLimit  string
	parallel     int
	experimental bool
}

func (r *runner) execute(ctx context.Context, request *runRequest, limits runLimits) (result runResult) {
	result = runResult{request: request}
	started := time.Now()
	defer func() { result.elapsed = time.Since(started) }()

	var err error
	result.overlay, err = writeOverlay(r.temporary, request.texts)
	if err != nil {
		result.outcome, result.err = outcomeFailed, err
		return result
	}
	overlayFile := ""
	if result.overlay != nil {
		overlayFile = result.overlay.file
	}
	directory, patterns, err := r.target(ctx, request, overlayFile, &result)
	if err != nil {
		result.outcome, result.err = failureOutcome(ctx), err
		return result
	}
	if len(patterns) == 0 {
		return result // nothing imports the changed packages
	}
	arguments := []string{"vet", "-vettool=" + r.executable, "-json"}
	if overlayFile != "" {
		arguments = append(arguments, "-overlay="+overlayFile)
	}
	if request.kind != runPackage {
		arguments = append(arguments, "-p="+strconv.Itoa(limits.parallel))
	}
	if limits.experimental {
		arguments = append(arguments, "-tier=experimental")
	}
	arguments = append(arguments, patterns...)
	result.commandLine = "go " + shellJoin(arguments)
	r.vet(ctx, directory, arguments, limits, &result)
	return result
}

// target resolves what a request checks: the working directory, the package
// patterns, and the directories whose findings the result will replace.
func (r *runner) target(ctx context.Context, request *runRequest, overlayFile string, result *runResult) (string, []string, error) {
	switch request.kind {
	case runPackage:
		result.covered = request.directories
		return request.directories[0], []string{"."}, nil
	case runWorkspace:
		// The run answers for every package the module lists, and for the
		// directories it already had findings for, which may be gone now.
		listed, err := listPackages(ctx, request.root, overlayFile, []string{"./..."})
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
		directories, err := importers(ctx, request.root, overlayFile, request.directories)
		if err != nil {
			return "", nil, err
		}
		result.covered = directories
		return request.root, relativePatterns(request.root, directories), nil
	}
}

func (r *runner) vet(ctx context.Context, directory string, arguments []string, limits runLimits, result *runResult) {
	runContext, cancel := context.WithTimeout(ctx, limits.timeout)
	defer cancel()
	command := exec.CommandContext(runContext, "go", arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GOMEMLIMIT="+limits.memoryLimit)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	configureProcessTree(command)
	if err := command.Start(); err != nil {
		result.outcome, result.err = outcomeFailed, err
		return
	}
	var exceeded atomic.Bool
	var peak atomic.Int64
	watchDone := make(chan struct{})
	go watchMemory(runContext, command.Process.Pid, limits.memoryBytes, &peak, func() {
		exceeded.Store(true)
		cancel()
	}, watchDone)
	waitErr := command.Wait()
	cancel()
	<-watchDone
	result.peakBytes = peak.Load()
	if largest, ok := largestProcessMemory(command.ProcessState); ok && largest > result.peakBytes {
		result.peakBytes = largest
	}
	result.stderr = stderr.String()
	switch {
	case exceeded.Load():
		result.outcome = outcomeMemoryExceeded
	case ctx.Err() != nil:
		result.outcome = outcomeCanceled
	case errors.Is(runContext.Err(), context.DeadlineExceeded):
		result.outcome = outcomeTimedOut
	default:
		decodeVetOutput(stdout.Bytes(), waitErr, result)
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
	result.diagnostics, result.analysisErrors = diagnostics, analysisErrors
	if waitErr != nil {
		// Some packages were analyzed and others failed to build. Which ones
		// failed is not recoverable from the output, so nothing is replaced.
		result.outcome, result.err = outcomeBuildFailed, waitErr
	}
}

func failureOutcome(ctx context.Context) runOutcome {
	if ctx.Err() != nil {
		return outcomeCanceled
	}
	return outcomeFailed
}

// watchMemory samples the child process tree until the run ends and calls
// stop when it passes the limit. Where the platform offers no measurement the
// watch does nothing: the soft GOMEMLIMIT and the timeout remain.
func watchMemory(ctx context.Context, pid int, limit int64, peak *atomic.Int64, stop func(), done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		used, measured := processTreeMemory(pid)
		if !measured {
			continue
		}
		if used > peak.Load() {
			peak.Store(used)
		}
		if limit > 0 && used > limit {
			stop()
			return
		}
	}
}
