package docprocessownership

import (
	"context"
	"os/exec"
)

//gohawk:example flagged
func run(ctx context.Context) error {
	command := exec.CommandContext(ctx, "worker")
	return command.Start() // want "started command is not waited on every successful return path"
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
