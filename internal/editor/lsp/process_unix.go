//go:build !windows

package lsp

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

// configureProcessTree puts go vet in its own process group so cancelling a
// run also stops the gohawk tool processes go vet started.
func configureProcessTree(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.WaitDelay = 2 * time.Second
}

// largestProcessMemory returns the peak resident size of the largest single
// process in the finished tree, as the kernel accounted it. It is a lower
// bound on what the tree used at once, and it is available even for a run too
// short to sample. The second result is false when the kernel reported none.
func largestProcessMemory(state *os.ProcessState) (int64, bool) {
	usage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok || usage.Maxrss <= 0 {
		return 0, false
	}
	peak := int64(usage.Maxrss) //nolint:unconvert // Maxrss is 32 bits wide on 32-bit platforms
	// Linux counts kilobytes; macOS counts bytes.
	if runtime.GOOS == "darwin" {
		return peak, true
	}
	return peak * 1024, true
}
