package processownership

import (
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
)

func impossibleSuccessfulStart(t *testing.T, ctx context.Context) {
	command := exec.CommandContext(ctx, "missing-tool")
	if err := command.Start(); err == nil {
		t.Fatal("expected Start to fail")
	}
}

func impossibleSuccessfulStartWithPipes(t *testing.T) {
	command := exec.Command("missing-tool")
	if _, err := command.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if _, err := command.StdoutPipe(); err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err == nil {
		t.Fatal("expected Start to fail")
	}
}

func settleCommand(command *exec.Cmd) error {
	return command.Wait()
}

func helperOwnsWait(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	return settleCommand(command)
}

func explicitlyDetached(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

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

func ownerRegisteredBeforeStart(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	defer func() { _ = command.Wait() }()
	return command.Start()
}

func returnedOwnerRegisteredBeforeStart(ctx context.Context) (func() error, error) {
	command := exec.CommandContext(ctx, "tool")
	wait := func() error { return command.Wait() }
	if err := command.Start(); err != nil {
		return nil, err
	}
	return wait, nil
}

func callerOwnsWait(command *exec.Cmd) bool {
	return command.Start() == nil
}

func callerOwnsSliceWait(commands []*exec.Cmd) error {
	for _, command := range commands {
		if err := command.Start(); err != nil {
			return err
		}
	}
	return nil
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

func startWaiter(command *exec.Cmd) {
	go func() { _ = command.Wait() }()
}

func waitedByHelperGoroutine(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	startWaiter(command)
	return nil
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

type commandController struct{ command *exec.Cmd }

func newCommandController(command *exec.Cmd) (*commandController, error) {
	return &commandController{command: command}, nil
}

func (controller *commandController) close() error { return nil }
func (controller *commandController) wait() error  { return controller.command.Wait() }

func controllerWatcherOwnsWait(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	controller, err := newCommandController(command)
	if err != nil {
		return err
	}
	defer func() { _ = controller.close() }()
	if err := command.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- controller.wait() }()
	return <-done
}

func controllerWithoutWatcher(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	controller, err := newCommandController(command)
	if err != nil {
		return err
	}
	defer func() { _ = controller.close() }()
	return command.Start() // want "started command is not waited on every successful return path"
}
