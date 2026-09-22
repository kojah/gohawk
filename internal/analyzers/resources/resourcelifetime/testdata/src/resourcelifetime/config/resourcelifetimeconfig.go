package resourcelifetimeconfig

import (
	"bytes"
	"compress/gzip"
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
