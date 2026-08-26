package processownership

import (
	"context"
	"os"
	"os/exec"
)

func orphan(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	return command.Start() // want "started command is not waited on every successful return path"
}

func owned(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	return command.Wait()
}

func ownedThroughAlias(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	alias := command
	if err := command.Start(); err != nil {
		return err
	}
	return alias.Wait()
}

func conditionallyOwned(ctx context.Context, wait bool) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	if wait {
		return command.Wait()
	}
	return nil
}

func deferredOwner(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	defer func() { _ = command.Wait() }()
	return nil
}

func callerOwnsWait(command *exec.Cmd) bool {
	return command.Start() == nil
}

func processExitOwnsTermination(ctx context.Context) {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return
	}
	os.Exit(1)
}
