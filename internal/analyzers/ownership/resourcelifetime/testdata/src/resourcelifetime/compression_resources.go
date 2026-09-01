package resourcelifetime

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
)

func leakedGzipWriter(destination io.Writer) {
	writer := gzip.NewWriter(destination) // want "owned resource from gzip.NewWriter is not released on every return path"
	_ = writer
}

func closedGzipReader() error {
	reader, err := gzip.NewReader(bytes.NewReader(nil))
	if err != nil {
		return err
	}
	defer reader.Close()
	return nil
}

func leakedZlibWriter(destination io.Writer) {
	writer := zlib.NewWriter(destination) // want "owned resource from zlib.NewWriter is not released on every return path"
	_ = writer
}
