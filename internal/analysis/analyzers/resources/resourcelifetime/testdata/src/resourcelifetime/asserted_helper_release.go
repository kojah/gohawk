package resourcelifetime

// A helper that closes its writer under a comma-ok assertion of io.Closer
// closes a file handed to it: the assertion holds for the file's type, so
// the arm without the close is not the file's path. A helper asserting a type
// the file does not satisfy leaves the release conditional. Both forms call
// the helper directly; a deferred helper is credited by the deferred-cleanup
// policy regardless of its condition.

import (
	"io"
	"os"
)

func maybeClose(writer io.Writer) {
	if closer, ok := writer.(io.Closer); ok {
		_ = closer.Close()
	}
}

type flusherCloser interface {
	io.Closer
	Flush() error
}

func maybeFlushClose(writer io.Writer) {
	if closer, ok := writer.(flusherCloser); ok {
		_ = closer.Close()
	}
}

func closedByAssertingHelper(path string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	maybeClose(file)
	return nil
}

func closedByUnsatisfiedAssertingHelper(path string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) // want "owned resource from os.OpenFile is not released on every return path"
	if err != nil {
		return err
	}
	maybeFlushClose(file)
	return nil
}
