package resourcelifetime

// Accepted gap: the pre-existing global-transfer model can mistake a stored
// scalar observation for retention. This boundary handles one wrapper around
// a positively contained aggregate; longer constructor chains count only at a
// callee proven to store them (see logger_chains.go), rather than treating
// arbitrary data dependence as ownership.

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
)

func installMultiWriter(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	log.SetOutput(io.MultiWriter(os.Stdout, file))
	return nil
}

func installWriterOrClose(path string, install bool) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if install {
		log.SetOutput(file)
		return nil
	}
	return file.Close()
}

func installWriterThenMaybeClose(path string, abandon bool) error {
	file, err := os.Create(path) // want "owned resource from os.Create is not released"
	if err != nil {
		return err
	}
	log.SetOutput(file)
	if abandon {
		return nil
	}
	return file.Close()
}

func discardMultiWriter(path string) error {
	file, err := os.Create(path) // want "owned resource from os.Create is not released"
	if err != nil {
		return err
	}
	_ = io.MultiWriter(os.Stdout, file)
	return nil
}

func decodeResponseWithoutClose(url string) (map[string]any, error) {
	response, err := http.Get(url) // want "owned resource from http.Get is not released"
	if err != nil {
		return nil, err
	}
	var value map[string]any
	err = json.NewDecoder(response.Body).Decode(&value)
	return value, err
}

var recordedStatus any

func rememberStatus(value any) { recordedStatus = value }

func recordResponseStatusWithoutClose(url string) error {
	response, err := http.Get(url) // want "owned resource from http.Get is not released"
	if err != nil {
		return err
	}
	rememberStatus(response.StatusCode)
	return nil
}

func decodeTransformedResponse(url string) (map[string]any, error) {
	response, err := http.Get(url) // want "owned resource from http.Get is not released"
	if err != nil {
		return nil, err
	}
	return decodeObject(response.Body)
}

func decodeObject(reader io.Reader) (map[string]any, error) {
	var result map[string]any
	err := json.NewDecoder(reader).Decode(&result)
	return result, err
}

func installUnrelatedWriter(path string) error {
	file, err := os.Create(path) // want "owned resource from os.Create is not released"
	if err != nil {
		return err
	}
	log.SetOutput(unrelatedWriter([]io.Writer{file}))
	return nil
}

func unrelatedWriter(_ []io.Writer) io.Writer { return io.Discard }

func scanWithoutClose(path string) int {
	file, _ := os.Open(path) // want "owned resource from os.Open is not released"
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count += len(scanner.Text())
	}
	return count
}
