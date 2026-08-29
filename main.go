// Command gohawk runs the gohawk static analyzers.
package main

import (
	"os"

	"github.com/kojah/gohawk/internal/cli"
)

func main() {
	// Process termination stays at the executable boundary. Analyzer and
	// reusable CLI packages return errors or exit codes to their caller.
	os.Exit(cli.Main())
}
