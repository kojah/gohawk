package after

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
)

// Apply uses Docker's fix: let os/exec manage the streams and process wait.
// Analyze this function; do not execute it against a real firewall.
func Apply(ctx context.Context, rules []byte) error {
	cmd := exec.CommandContext(ctx, "nft", "-f", "-")
	var stdout, stderr strings.Builder
	cmd.Stdin = bytes.NewReader(rules)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	return cmd.Run()
}
