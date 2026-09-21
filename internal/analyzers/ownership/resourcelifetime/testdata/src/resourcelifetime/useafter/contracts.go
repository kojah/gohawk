package useafter

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
)

func resetThroughInterface(output io.Writer) {
	writer := gzip.NewWriter(output)
	_ = writer.Close()
	var resetter interface{ Reset(io.Writer) } = writer
	resetter.Reset(output)
	_, _ = writer.Write([]byte("fresh"))
	_ = writer.Close()
}

func closedMemoryWriter() {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	_ = writer.Close()
	_, _ = writer.Write([]byte("late")) // want "resource from gzip.NewWriter is used after Close"
}

type harmlessWriter struct{}

func (*harmlessWriter) Close() error              { return nil }
func (*harmlessWriter) Write([]byte) (int, error) { return 0, nil }

func notAnInvalidationContract() {
	writer := &harmlessWriter{}
	_ = writer.Close()
	_, _ = writer.Write(nil)
}

func resetWriter(output io.Writer) {
	writer := gzip.NewWriter(output)
	_ = writer.Close()
	writer.Reset(output)
	_, _ = writer.Write([]byte("fresh"))
	_ = writer.Close()
}

// Closing a gzip reader does not establish that subsequent Read must fail.
func readerClose(input io.Reader) {
	reader, err := gzip.NewReader(input)
	if err != nil {
		return
	}
	_ = reader.Close()
	_, _ = reader.Read(make([]byte, 1))
}

func replacedBody(client *http.Client, request *http.Request, fresh io.ReadCloser) {
	response, err := client.Do(request)
	if err != nil {
		return
	}
	_ = response.Body.Close()
	response.Body = fresh
	_, _ = response.Body.Read(make([]byte, 1))
	_ = response.Body.Close()
}

func savedBody(client *http.Client, request *http.Request) {
	response, err := client.Do(request)
	if err != nil {
		return
	}
	body := response.Body
	_ = body.Close()
	_, _ = body.Read(make([]byte, 1)) // want "resource from http.Do is used after Close"
}

func closedWriter(output io.Writer) {
	writer := gzip.NewWriter(output)
	_ = writer.Close()
	_, _ = writer.Write([]byte("late")) // want "resource from gzip.NewWriter is used after Close"
}
