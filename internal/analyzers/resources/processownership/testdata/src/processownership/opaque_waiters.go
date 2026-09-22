package processownership

import (
	"os"
	"os/exec"
)

var registerWaiter func(func() error)

func opaqueCallbackMayWait() error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil {
		return err
	}
	registerWaiter(func() error { return cmd.Wait() })
	return nil
}

func otherCallbackDoesNotWait() error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil { // want "started command is not waited"
		return err
	}
	other := exec.Command("other")
	registerWaiter(func() error { return other.Wait() })
	return cmd.Process.Kill()
}

func waiterExitsProcess() error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
		os.Exit(0)
	}()
	return nil
}

type commandValueOwner struct{ command *exec.Cmd }

func (owner commandValueOwner) Close() error { return owner.command.Wait() }

func valueOwnerWithLoadedCommand() (commandValueOwner, error) {
	var owner commandValueOwner
	owner.command = exec.Command("tool")
	owner.command.Stdout = os.Stdout
	if _, err := owner.command.StdinPipe(); err != nil {
		return commandValueOwner{}, err
	}
	if err := owner.command.Start(); err != nil {
		return commandValueOwner{}, err
	}
	return owner, nil
}
