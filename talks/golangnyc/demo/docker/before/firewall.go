package before

import (
	"context"
	"io"
	"os/exec"
	"strings"
)

// Apply is a reduced example of Docker's nftables subprocess handling.
// Analyze this function; do not execute it against a real firewall.
func Apply(ctx context.Context, rules []byte) error {
	cmd := exec.CommandContext(ctx, "nft", "-f", "-")
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	errors, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	if _, err := input.Write(rules); err != nil {
		return err // The process started, but this path skips Wait.
	}
	if err := input.Close(); err != nil {
		return err
	}
	var stdout, stderr strings.Builder
	if _, err := io.Copy(&stdout, output); err != nil {
		return err
	}
	if _, err := io.Copy(&stderr, errors); err != nil {
		return err
	}
	return cmd.Wait()
}
