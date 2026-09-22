package processownership

import "os/exec"

// Callback identity must reach the exact command. Merely invoking a callback,
// or waiting for another command, does not discharge the Start obligation.
func invokeCommandCallback(command *exec.Cmd, fn func(*exec.Cmd)) {
	func() { fn(command) }()
}

func callbackWaitsForCommand() error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil {
		return err
	}
	invokeCommandCallback(command, func(c *exec.Cmd) { c.Wait() })
	return nil
}

func callbackIgnoresCommand() error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	invokeCommandCallback(command, func(c *exec.Cmd) {})
	return nil
}

func callbackWaitsForOtherCommand(other *exec.Cmd) error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	invokeCommandCallback(command, func(c *exec.Cmd) { other.Wait() })
	return nil
}

type commandCallbacks struct{ wait func(*exec.Cmd) }

func invokeCommandField(callbacks *commandCallbacks, command *exec.Cmd) { callbacks.wait(command) }

func callbackFieldWaitsForCommand() error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil {
		return err
	}
	invokeCommandField(&commandCallbacks{wait: func(c *exec.Cmd) { c.Wait() }}, command)
	return nil
}

func callbackFieldIgnoresCommand() error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	invokeCommandField(&commandCallbacks{wait: func(c *exec.Cmd) {}}, command)
	return nil
}
