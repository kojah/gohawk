//go:build windows

package lsp

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// configureProcessTree makes cancelling a run stop go vet together with the
// gohawk tool processes it started. Windows has no process groups to signal,
// so taskkill walks the tree.
func configureProcessTree(command *exec.Cmd) {
	command.Cancel = func() error {
		kill := exec.CommandContext(context.Background(), "taskkill", "/T", "/F", "/PID", strconv.Itoa(command.Process.Pid))
		if err := kill.Run(); err != nil {
			return command.Process.Kill()
		}
		return nil
	}
	command.WaitDelay = 2 * time.Second
}

// largestProcessMemory reports that Windows offers no figure for the tree
// through the process state.
func largestProcessMemory(*os.ProcessState) (int64, bool) {
	return 0, false
}
