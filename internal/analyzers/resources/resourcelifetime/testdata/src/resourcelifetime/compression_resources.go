package resourcelifetime

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"os"
)

// Reader Close is not an owned-resource obligation. The underlying input is
// still independently owned, and its missing Close must remain reportable.
func borrowedCompressionReaders(input io.Reader) {
	reader, err := gzip.NewReader(input)
	if err == nil {
		_, _ = io.ReadAll(reader)
	}
	zreader, err := zlib.NewReader(input)
	if err == nil {
		_, _ = io.ReadAll(zreader)
	}
	dictReader, err := zlib.NewReaderDict(input, nil)
	if err == nil {
		_, _ = io.ReadAll(dictReader)
	}
}

func compressionDoesNotOwnFile(path string) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	reader, err := gzip.NewReader(file)
	if err == nil {
		_ = reader.Close()
	}
}

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

func failedCompression(destination io.Writer, input io.Reader) error {
	writer := gzip.NewWriter(destination)
	if _, err := io.Copy(writer, input); err != nil {
		return err
	}
	return writer.Close()
}

func unfinishedSuccessfulCompression(destination io.Writer) error {
	writer := gzip.NewWriter(destination) // want "owned resource from gzip.NewWriter is not released"
	_, _ = writer.Write([]byte("unfinished"))
	return nil
}

func abortedCompressionPipe(output *io.PipeWriter, failure error) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("partial"))
	_ = output.CloseWithError(failure)
}

func capturedAbortedPipe(output *io.PipeWriter, failure error, wait <-chan struct{}) {
	go func() {
		writer := gzip.NewWriter(output)
		_, _ = writer.Write([]byte("partial"))
		<-wait
		_ = output.CloseWithError(failure)
	}()
}

func unrelatedAbortedPipe(output, other *io.PipeWriter, failure error) {
	writer := gzip.NewWriter(output) // want "owned resource from gzip.NewWriter is not released"
	_, _ = writer.Write([]byte("partial"))
	_ = other.CloseWithError(failure)
}

func successfullyClosedPipe(output *io.PipeWriter) {
	writer := gzip.NewWriter(output) // want "owned resource from gzip.NewWriter is not released"
	_, _ = writer.Write([]byte("partial"))
	_ = output.CloseWithError(nil)
}

// A writer over a local in-memory buffer is exempt unless
// -require-memory-writer-close is set; nothing outside the function is held.
func gzipWriterOverLocalBuffer(data []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func zlibWriterOverConstructedBuffer(dst, src []byte) []byte {
	buffer := bytes.NewBuffer(dst[:0])
	writer := zlib.NewWriter(buffer)
	if _, err := writer.Write(src); err != nil {
		return nil
	}
	if err := writer.Close(); err != nil {
		return nil
	}
	return buffer.Bytes()
}

func gzipWriterOverConstructedStringBuffer() {
	writer := gzip.NewWriter(bytes.NewBufferString("prefix"))
	_ = writer
}

func NewBuffer(destination io.Writer) io.Writer {
	return destination
}

func customBufferFactory(destination io.Writer) {
	writer := gzip.NewWriter(NewBuffer(destination)) // want "owned resource from gzip.NewWriter is not released"
	_ = writer
}

func mixedMemoryAndExternalWriter(destination io.Writer, external bool) {
	var output io.Writer = bytes.NewBuffer(nil)
	if external {
		output = destination
	}
	writer := gzip.NewWriter(output) // want "owned resource from gzip.NewWriter is not released"
	_ = writer
}
