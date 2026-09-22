package processownership

import "os/exec"

func successfulMergedCommand(run bool) error {
	var command *exec.Cmd
	if run {
		command = exec.Command("tool")
		if err := command.Start(); err != nil {
			return err
		}
	}
	println("other work")
	if command != nil {
		return command.Wait()
	}
	return nil
}

func mergedDifferentCommand(run bool) error {
	var command *exec.Cmd
	if run {
		started := exec.Command("tool")
		if err := started.Start(); err != nil { // want "started command is not waited on every successful return path"
			return err
		}
		command = exec.Command("other")
		started.Process.Kill()
	}
	if command != nil {
		return command.Wait()
	}
	return nil
}

func mergedCommandDropped(run, drop bool) error {
	var command *exec.Cmd
	if run {
		command = exec.Command("tool")
		if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
			return err
		}
	}
	if drop {
		if command != nil {
			command.Process.Kill()
		}
		command = nil
	}
	if command != nil {
		return command.Wait()
	}
	return nil
}

func deferredProcessGuardAndFlag(wait bool) error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	defer func() {
		if command.Process != nil && wait {
			command.Wait()
		}
	}()
	return nil
}

func deferredProcessGuardCleared() error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	defer func() {
		command.Process = nil
		if command.Process != nil {
			command.Wait()
		}
	}()
	return nil
}
