package lsp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

	// One deadline and one memory watch cover everything the run starts: the
	// package listing as well as the analysis.
	runContext, cancel := context.WithTimeout(ctx, limits.timeout)
	defer cancel()
	guarded := &guard{limits: limits, stop: cancel}

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
	arguments := []string{"vet", "-vettool=" + r.executable, "-json", "-p=" + strconv.Itoa(limits.parallel)}
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

// interrupted names why a run stopped early, or returns otherwise when it was
// not stopped. A memory stop also cancels the run context, so it is asked
// first; a cancellation by the caller outranks the run's own deadline.
func interrupted(caller, run context.Context, guarded *guard, otherwise runOutcome) runOutcome {
	switch {
	case guarded.exceeded.Load():
		return outcomeMemoryExceeded
	case caller.Err() != nil:
		return outcomeCanceled
	case errors.Is(run.Err(), context.DeadlineExceeded):
		return outcomeTimedOut
	default:
		return otherwise
	}
}

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
		result.covered = directories
		return request.root, relativePatterns(request.root, directories), nil
	}
}

// guard runs go commands under one run's limits. Every command joins the
// same memory accounting, and passing the limit stops the whole run.
type guard struct {
	limits   runLimits
	stop     context.CancelFunc
	exceeded atomic.Bool
	peak     atomic.Int64
}

type commandOutput struct {
	stdout    []byte
	stderr    []byte
	truncated bool
}

// maxCommandOutput bounds what one command may print. Findings are held in
// memory until they are published, so the output is part of the server's
// own footprint.
const maxCommandOutput = 64 << 20

func (g *guard) run(ctx context.Context, directory string, arguments []string) (commandOutput, error) {
	return g.runProgram(ctx, directory, "go", arguments)
}

// runProgram runs any program under the limits; the go command is the usual
// one, and the explanation runs gohawk itself.
func (g *guard) runProgram(ctx context.Context, directory, program string, arguments []string) (commandOutput, error) {
	command := exec.CommandContext(ctx, program, arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GOMEMLIMIT="+g.limits.memoryLimit, ServerProcessVariable+"="+strconv.Itoa(os.Getpid()))
	stdout := &cappedBuffer{limit: maxCommandOutput}
	stderr := &cappedBuffer{limit: 1 << 20}
	command.Stdout, command.Stderr = stdout, stderr
	configureProcessTree(command)
	if err := command.Start(); err != nil {
		return commandOutput{}, err
	}
	watchContext, stopWatch := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go watchMemory(watchContext, command.Process.Pid, g.limits.memoryBytes, &g.peak, func() {
		g.exceeded.Store(true)
		g.stop()
	}, watchDone)
	waitErr := command.Wait()
	stopWatch()
	<-watchDone
	if largest, ok := largestProcessMemory(command.ProcessState); ok && largest > g.peak.Load() {
		g.peak.Store(largest)
	}
	return commandOutput{stdout: stdout.buffer.Bytes(), stderr: stderr.buffer.Bytes(), truncated: stdout.truncated}, waitErr
}

// cappedBuffer keeps the first limit bytes and remembers that more arrived.
type cappedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	if room := b.limit - b.buffer.Len(); room < len(data) {
		b.truncated = true
		b.buffer.Write(data[:max(room, 0)])
		return len(data), nil
	}
	return b.buffer.Write(data)
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
