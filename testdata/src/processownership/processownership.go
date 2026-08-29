package processownership

import (
	"context"
	"io"
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

func ownedStdoutPipe(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	pipe, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, pipe)
	return command.Wait()
}

func transferredCommandAndPipe(ctx context.Context) (*exec.Cmd, io.ReadCloser, error) {
	command := exec.CommandContext(ctx, "tool")
	pipe, err := command.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := command.Start(); err != nil {
		return nil, nil, err
	}
	return command, pipe, nil
}

func orphanedPipe(ctx context.Context) (io.ReadCloser, error) {
	command := exec.CommandContext(ctx, "tool")
	pipe, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return nil, err
	}
	return pipe, nil
}

func transferredWaitClosure(ctx context.Context) (func() error, error) {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return nil, err
	}
	return func() error { return command.Wait() }, nil
}

func waitedInGoroutine(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	return <-done
}

type processOwner struct {
	command *exec.Cmd
}

func returnedProcessOwner(ctx context.Context) (*processOwner, error) {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &processOwner{command: command}, nil
}

func (owner *processOwner) startOwnedCommand() error {
	return owner.command.Start()
}

func transferredToProcessOwner(ctx context.Context, owner *processOwner) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	owner.command = command
	return nil
}

func preownedCommand(ctx context.Context, cleanup func(func())) *exec.Cmd {
	command := exec.CommandContext(ctx, "tool")
	cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	})
	return command
}

func startsPreownedCommand(ctx context.Context, cleanup func(func())) error {
	command := preownedCommand(ctx, cleanup)
	return command.Start()
}

func startsAddressablePreownedCommand(ctx context.Context, cleanup func(func())) error {
	command := preownedCommand(ctx, cleanup)
	cleanup(func() { _ = command.Process.Kill() })
	return command.Start()
}
