package processownership

import (
	"context"
	"os/exec"
)

//gohawk:example flagged
func run(ctx context.Context, wait bool) error {
	command := exec.CommandContext(ctx, "worker")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	if wait {
		return command.Wait()
	}
	return nil
}

//gohawk:example end

//gohawk:example ok
func runSafely(ctx context.Context) error {
	command := exec.CommandContext(ctx, "worker")
	if err := command.Start(); err != nil {
		return err
	}
	return command.Wait()
}

//gohawk:example end
