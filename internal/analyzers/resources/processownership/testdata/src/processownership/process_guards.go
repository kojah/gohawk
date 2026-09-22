package processownership

import "os/exec"

// A direct successful Start establishes a non-nil Process. More distant guards
// remain outside this narrow feasibility rule because callers can mutate Cmd.
func releaseImmediatelyGuardedProcess() error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil {
		return err
	}
	if command.Process != nil {
		return command.Process.Release()
	}
	return nil
}

func waitAfterImpossibleNilProcessReturn() error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil {
		return err
	}
	if command.Process == nil {
		return nil
	}
	return command.Wait()
}

func overwrittenProcessMustNotProveWait() error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	command.Process = nil
	if command.Process != nil {
		return command.Wait()
	}
	return nil
}

func otherCommandProcessMustNotProveWait(other *exec.Cmd) error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	if other.Process != nil {
		return command.Wait()
	}
	return nil
}

func nonNilProcessStillNeedsWait(enabled bool) error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	if command.Process == nil {
		return nil
	}
	if enabled {
		return command.Wait()
	}
	return nil
}
