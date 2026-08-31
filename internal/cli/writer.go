package cli

import (
	"fmt"
	"io"
)

// CLI presentation is best-effort: once the selected output writer fails,
// there is no reliable alternate stream on which to report that failure. The
// command's exit status remains reserved for usage and analysis outcomes.
func writeLine(output io.Writer, values ...any) {
	_, _ = fmt.Fprintln(output, values...)
}

func writeFormatted(output io.Writer, format string, values ...any) {
	_, _ = fmt.Fprintf(output, format, values...)
}
