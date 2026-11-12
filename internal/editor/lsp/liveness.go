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
