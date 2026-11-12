// Command gohawk runs the gohawk static analyzers.
package main

import (
	"os"

	"github.com/kojah/gohawk/internal/cli"
)

func main() {
	// An analysis started by an editor server has no reader once that server
	// is gone. Stopping here keeps a killed editor from leaving analyses
	// running; it is a no-op for every other way gohawk is started.
	if gone := cli.EditorServerGone(); gone != nil {
		go func() {
			<-gone
			os.Exit(1)
		}()
	}
	// Process termination stays at the executable boundary. Analyzer and
	// reusable CLI packages return errors or exit codes to their caller.
	os.Exit(cli.Main())
}
