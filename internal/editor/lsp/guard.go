package lsp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Limits on what one run may use, enforced on every process it starts. A run
// is stopped when its process tree passes the memory limit, when it goes too
// long without progress, or when it outlasts the longest run allowed. Each
// stop is reported as a stop, never as a clean result.
//
// Progress has two sources. The go command names each package it compiles
// (-v), and the analysis tool appends a line to the run's progress file for
// each package it starts analyzing. Either one resets the idle clock. That
// makes the time limit a bound on one package's work rather than on the whole
// run, so a first check on a large repository, which must compile and analyze
// every dependency once, is not cut off while it is getting somewhere.

// runProgress is what a run has done so far, read by the status line while
// the run is still going.
type runProgress struct {
	packages atomic.Int64 // packages the analysis tool has started on
	last     atomic.Int64 // UnixNano of the last sign of progress
	started  time.Time
	// file is where the analysis tool reports; read is how much of it has
	// been counted. Only the watch goroutine of the running command reads.
	file string
	read int64
}

// collect counts the packages reported since the last call.
func (p *runProgress) collect() {
	if p.file == "" {
		return
	}
	info, err := os.Stat(p.file)
	if err != nil || info.Size() <= p.read {
		return
	}
	file, err := os.Open(p.file)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	fresh := make([]byte, info.Size()-p.read)
	count, _ := file.ReadAt(fresh, p.read)
	started := bytes.Count(fresh[:count], []byte("\n"))
	if started == 0 {
		return
	}
	// Advance past whole lines only, so a line caught mid-write is counted
	// on the next pass.
	p.read += int64(bytes.LastIndexByte(fresh[:count], '\n') + 1)
	p.packages.Add(int64(started))
	p.touch()
}

func newRunProgress() *runProgress {
	progress := &runProgress{started: time.Now()}
	progress.last.Store(progress.started.UnixNano())
	return progress
}

func (p *runProgress) touch() {
	p.last.Store(time.Now().UnixNano())
}

func (p *runProgress) idleFor() time.Duration {
	return time.Since(time.Unix(0, p.last.Load()))
}

// guard runs commands under one run's limits. Every command joins the same
// memory accounting, and passing a limit stops the whole run.
type guard struct {
	limits   runLimits
	stop     context.CancelFunc
	exceeded atomic.Bool
	stalled  atomic.Bool
	peak     atomic.Int64
	progress *runProgress
}

func newGuard(limits runLimits, stop context.CancelFunc, progress *runProgress) *guard {
	if progress == nil {
		progress = newRunProgress()
	}
	return &guard{limits: limits, stop: stop, progress: progress}
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
	// Each command gets the full idle allowance, however long the one
	// before it took.
	g.progress.touch()
	command := exec.CommandContext(ctx, program, arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(),
		"GOMEMLIMIT="+g.limits.memoryLimit,
		ServerProcessVariable+"="+strconv.Itoa(os.Getpid()),
		ProgressFileVariable+"="+g.progress.file,
	)
	stdout := &cappedBuffer{limit: maxCommandOutput}
	stderr := &progressWriter{progress: g.progress, rest: cappedBuffer{limit: 1 << 20}}
	command.Stdout, command.Stderr = stdout, stderr
	configureProcessTree(command)
	if err := command.Start(); err != nil {
		return commandOutput{}, err
	}
	watchContext, stopWatch := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go g.watch(watchContext, command.Process.Pid, watchDone)
	waitErr := command.Wait()
	stopWatch()
	<-watchDone
	if largest, ok := largestProcessMemory(command.ProcessState); ok && largest > g.peak.Load() {
		g.peak.Store(largest)
	}
	return commandOutput{stdout: stdout.buffer.Bytes(), stderr: stderr.rest.buffer.Bytes(), truncated: stdout.truncated}, waitErr
}

// watch samples the command until it ends and stops the run when a limit is
// passed. Where the platform offers no memory measurement only the idle
// limit is watched: the soft GOMEMLIMIT remains.
func (g *guard) watch(ctx context.Context, pid int, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		g.progress.collect()
		if g.limits.idle > 0 && g.progress.idleFor() > g.limits.idle {
			g.stalled.Store(true)
			g.stop()
			return
		}
		used, measured := processTreeMemory(pid)
		if !measured {
			continue
		}
		if used > g.peak.Load() {
			g.peak.Store(used)
		}
		if g.limits.memoryBytes > 0 && used > g.limits.memoryBytes {
			g.exceeded.Store(true)
			g.stop()
			return
		}
	}
}

// interrupted names why a run stopped early, or returns otherwise when it was
// not stopped. The run's own stops also cancel its context, so they are asked
// first; a cancellation by the caller outranks the longest-run deadline.
func interrupted(caller, run context.Context, guarded *guard, otherwise runOutcome) runOutcome {
	switch {
	case guarded.exceeded.Load():
		return outcomeMemoryExceeded
	case guarded.stalled.Load():
		return outcomeTimedOut
	case caller.Err() != nil:
		return outcomeCanceled
	case errors.Is(run.Err(), context.DeadlineExceeded):
		return outcomeTimedOut
	default:
		return otherwise
	}
}

// progressWriter reads a command's standard error. With -v the go command
// prints the import path of each package it compiles, one per line; those
// lines reset the idle clock and are dropped. Everything else, which is what
// a build failure prints, is kept for the log.
type progressWriter struct {
	progress *runProgress
	partial  []byte
	rest     cappedBuffer
}

func (w *progressWriter) Write(data []byte) (int, error) {
	w.partial = append(w.partial, data...)
	for {
		end := bytes.IndexByte(w.partial, '\n')
		if end < 0 {
			break
		}
		line := string(w.partial[:end])
		w.partial = w.partial[end+1:]
		if isPackageLine(line) {
			// A compiled package is progress, but it is not counted: the
			// count shown to the user is packages analyzed.
			w.progress.touch()
			continue
		}
		_, _ = w.rest.Write([]byte(line + "\n"))
	}
	// A line with no end in sight is not a package name; do not hold it.
	if len(w.partial) > 4096 {
		_, _ = w.rest.Write(w.partial)
		w.partial = nil
	}
	return len(data), nil
}

// isPackageLine reports whether a line of go command output names a package.
// A compiler or go command message always carries a colon (a file position
// or a "go:" prefix) or starts a "# package" header; an import path, with an
// optional test variant in brackets, has neither.
func isPackageLine(line string) bool {
	line = strings.TrimSpace(line)
	return line != "" && !strings.HasPrefix(line, "#") && !strings.Contains(line, ":")
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
