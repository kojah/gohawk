package lsp

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// A server that is killed outright cannot stop the checks it started, and
// cannot remove its temporary files. Two things bound the damage. Every
// process a check starts is told which server owns it, so the analysis tool
// can stop when that server is gone. And a starting server removes the
// temporary directories of servers that no longer exist.

// ServerProcessVariable names the environment variable that carries the
// owning server's process ID to the processes a check starts.
const ServerProcessVariable = "GOHAWK_LSP_SERVER_PID"

// ProgressFileVariable names the environment variable that carries the path
// of a run's progress file. The analysis tool appends one line to it for
// each package it starts on. The go command only names a package when it
// compiles it, so without this a run that is analyzing dependencies looks
// idle to the server.
const ProgressFileVariable = "GOHAWK_LSP_PROGRESS"

// AnnounceAnalysis records, for the server that started this process, that
// the analysis of one package has begun. It does nothing when the process
// was not started by an editor server. A failure to record is ignored: the
// analysis matters more than its progress line.
func AnnounceAnalysis(packagePath string) {
	path := os.Getenv(ProgressFileVariable)
	if path == "" {
		return
	}
	//nolint:gosec // the progress file the owning server created and named for this run
	file, err := os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return
	}
	// One short append is written whole, so lines from tools running in
	// parallel do not mix.
	_, _ = file.WriteString(strings.ReplaceAll(packagePath, "\n", " ") + "\n")
	_ = file.Close()
}

const temporaryPrefix = "gohawk-lsp-"

// ProcessAlive reports whether a process with this ID exists. When the
// answer cannot be determined it reports true, so nothing is stopped or
// removed on a guess.
func ProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	defer func() { _ = process.Release() }()
	if runtime.GOOS == "windows" {
		// FindProcess opens the process on Windows and fails when it is gone.
		return true
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH)
}

// newTemporaryDirectory creates this server's directory, named after its
// process so a later server can tell whether the owner still exists.
func newTemporaryDirectory() (string, error) {
	return os.MkdirTemp("", temporaryPrefix+strconv.Itoa(os.Getpid())+"-")
}

// removeAbandonedDirectories deletes the temporary directories of servers
// that are gone. A directory whose owner cannot be read from its name is
// left alone.
func removeAbandonedDirectories() {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return
	}
	for _, entry := range entries {
		rest, found := strings.CutPrefix(entry.Name(), temporaryPrefix)
		if !found || !entry.IsDir() {
			continue
		}
		owner, _, _ := strings.Cut(rest, "-")
		pid, err := strconv.Atoi(owner)
		if err != nil || pid == os.Getpid() || ProcessAlive(pid) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(os.TempDir(), entry.Name()))
	}
}
