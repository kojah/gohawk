package resourcelifetime

import (
	"compress/gzip"
	"io"
)

type compressionFailure struct{}

func (*compressionFailure) Error() string { return "compression failed" }

// A typed nil error is a nonnil interface. Returning it or aborting a pipe
// abandons the output, just like any other error; this does not prove Close.
func failedTypedNilCompression(destination io.Writer) error {
	writer := gzip.NewWriter(destination)
	_, _ = writer.Write([]byte("partial"))
	return (*compressionFailure)(nil)
}

func abortedTypedNilCompression(output *io.PipeWriter) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("partial"))
	_ = output.CloseWithError((*compressionFailure)(nil))
}

func unfinishedNilErrorCompression(destination io.Writer) error {
	writer := gzip.NewWriter(destination) // want "owned resource from gzip.NewWriter is not released"
	_, _ = writer.Write([]byte("unfinished"))
	return nil
}
