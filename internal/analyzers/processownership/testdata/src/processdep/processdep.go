package processdep

import "os/exec"

func Wait(command *exec.Cmd) error { return command.Wait() }

func MaybeWait(command *exec.Cmd, enabled bool) error {
	if enabled {
		return command.Wait()
	}
	return nil
}
