package processownership

import (
	"os/exec"
)

// Synchronous standard IO operations alone do not identify a process wait owner.
// These launches may still leak: local IO-only use preserves the unused-command gap.
func pipeOnlyInput() error {
	cmd := exec.Command("tool")
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	_, _ = pipe.Write([]byte("payload"))
	return pipe.Close()
}

func pipeOnlyOutput() error {
	cmd := exec.Command("tool")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	_, _ = pipe.Read(make([]byte, 64))
	return pipe.Close()
}

func pipeOnlyErrorOutput() error {
	cmd := exec.Command("tool")
	pipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	_, _ = pipe.Read(make([]byte, 64))
	return pipe.Close()
}

func pipePartialWait(wait bool) error {
	cmd := exec.Command("tool")
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	_, _ = pipe.Write([]byte("payload"))
	_ = pipe.Close()
	if wait {
		return cmd.Wait()
	}
	return nil
}

func pipeThenKill() error {
	cmd := exec.Command("tool")
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	_ = pipe.Close()
	return cmd.Process.Kill()
}

type misleadingPipe struct{}

func (misleadingPipe) StdinPipe(cmd *exec.Cmd) *exec.Cmd { return cmd }

func pipeNamedCommand(wait bool) error {
	cmd := exec.Command("tool")
	owner := (misleadingPipe{}).StdinPipe(cmd)
	if err := cmd.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	if wait {
		return owner.Wait()
	}
	return nil
}
