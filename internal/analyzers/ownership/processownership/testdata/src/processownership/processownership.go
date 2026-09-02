package processownership

import (
	"context"
	"io"
	"os"
	"os/exec"
	"processdep"
	"testing"
)

func importedHelperOwnsWait(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	return processdep.Wait(command)
}

func conditionalImportedHelperDoesNotOwnWait(ctx context.Context, enabled bool) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	return processdep.MaybeWait(command, enabled)
}

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
	return command.Start() // want "started command is never waited on or released"
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

func waitedThroughDirectProcessHandle(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	_, _ = command.Process.Wait()
	return nil
}

func conditionallyWaitedThroughDirectProcessHandle(ctx context.Context, wait bool) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	if wait {
		_, _ = command.Process.Wait()
	}
	return nil
}

func reapedByDetachedGoroutine(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}

func reapedByDetachedGoroutineNestedDefer(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	go func() { defer func() { _ = command.Wait() }() }()
	return nil
}

func conditionallyReapedByDetachedGoroutine(ctx context.Context, reap bool) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	go func() {
		if reap {
			defer func() { _ = command.Wait() }()
		}
	}()
	return nil
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

type osProcessWaiter struct {
	done chan struct{}
}

type processFieldOwner struct {
	process *os.Process
}

func (owner *processFieldOwner) start(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	owner.process = command.Process
	return nil
}

func locallyStoredProcessHandle(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	owner := new(processFieldOwner)
	owner.process = command.Process
	return nil
}

func newOSProcessWaiter(process *os.Process) *osProcessWaiter {
	waiter := &osProcessWaiter{done: make(chan struct{})}
	go func() {
		_, _ = process.Wait()
		close(waiter.done)
	}()
	return waiter
}

func waitedThroughProcessHandle(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	_ = newOSProcessWaiter(command.Process)
	return nil
}

func waitThroughNestedProcessHelper(process *os.Process) {
	_ = newOSProcessWaiter(process)
}

func waitedThroughStartedWrapper(ctx context.Context) error {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return err
	}
	go waitThroughNestedProcessHelper(command.Process)
	return nil
}

func waitedLaterInLoop(ctx context.Context, commands []string) error {
	for _, name := range commands {
		command := exec.CommandContext(ctx, name)
		if err := command.Start(); err != nil {
			break
		}
		_ = command.Wait()
	}
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
	return command.Start() // want "started command is never waited on or released"
}

type startedCopy exec.Cmd

func (copy *startedCopy) Close() {
	_ = (*exec.Cmd)(copy).Process.Kill()
	_ = (*exec.Cmd)(copy).Wait()
}

// Returning a by-value copy of the started command hands the same process
// state, and therefore the wait obligation, to the caller.
func returnedCommandCopy(ctx context.Context) *startedCopy {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		panic(err)
	}
	copy := startedCopy(*command)
	return &copy
}

// Returning the started os.Process hands the caller a handle it can Wait on,
// so the reap obligation moves with it.
func returnedProcessHandle(ctx context.Context) (*os.Process, error) {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil {
		return nil, err
	}
	return command.Process, nil
}

func returnedProcessPidOnly(ctx context.Context) (int, error) {
	command := exec.CommandContext(ctx, "tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return 0, err
	}
	return command.Process.Pid, nil
}

type forwardWait struct {
	done chan struct{}
	err  error
}

// startForwardWait invokes the wait callback on its own goroutine.
func startForwardWait(wait func() error) *forwardWait {
	w := &forwardWait{done: make(chan struct{})}
	go func() {
		w.err = wait()
		close(w.done)
	}()
	return w
}

func storeForwardWait(wait func() error) *forwardWait {
	return &forwardWait{done: make(chan struct{}), err: nil}
}

// A helper that invokes the bound Wait on a launched goroutine owns reaping.
func commandWaitedByStartedCallback() error {
	cmd := exec.Command("ssh", "-N")
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = startForwardWait(cmd.Wait)
	return nil
}

func commandWaitDroppedByHelper() error {
	cmd := exec.Command("ssh", "-N")
	if err := cmd.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	_ = storeForwardWait(cmd.Wait)
	return nil
}

type envoyDriver struct {
	cmd  *exec.Cmd
	done chan error
}

// A command stored on the receiver before Start belongs to the receiver,
// which waits through the field later.
func (e *envoyDriver) start() error {
	cmd := exec.Command("envoy")
	e.cmd = cmd
	if err := cmd.Start(); err != nil {
		return err
	}
	e.done = make(chan error, 1)
	go func() { e.done <- e.cmd.Wait() }()
	return nil
}

// A command stored only in a local struct before Start is not transferred.
func startWithLocalHolder() error {
	holder := &envoyDriver{}
	cmd := exec.Command("envoy")
	holder.cmd = cmd
	if err := cmd.Start(); err != nil { // want "started command is never waited on or released"
		return err
	}
	return nil
}
