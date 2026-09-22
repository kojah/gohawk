package resourcelifetimeconfig

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"net/http"
	"os"
)

func ignoredFile() error {
	file, err := os.Open("fixture")
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func reportedResponse() error {
	response, err := http.Get("https://example.com") // want "owned resource from http.Get is not released"
	if err != nil {
		return err
	}
	_ = response
	return nil
}

func ignoredReader() error {
	reader, err := gzip.NewReader(bytes.NewReader(nil))
	if err != nil {
		return err
	}
	_ = reader
	return nil
}

// Strict mode keeps finalization obligations for both buffer construction forms.
func reportedMemoryWriters() {
	first := gzip.NewWriter(bytes.NewBuffer(nil))             // want "owned resource from gzip.NewWriter is not released"
	second := zlib.NewWriter(bytes.NewBufferString("prefix")) // want "owned resource from zlib.NewWriter is not released"
	_, _ = first, second
}
