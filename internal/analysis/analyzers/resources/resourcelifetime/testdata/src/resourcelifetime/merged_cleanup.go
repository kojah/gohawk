package resourcelifetime

import (
	"net/http"
	"os"
)

// A helper can release a merged response body even when exact owner projection
// is unavailable. The ambiguous projection is unknown, never proven cleanup.
func replacedResponseClosedByHelper(client *http.Client, request *http.Request, retry bool) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if retry {
		drainAndCloseBody(response.Body)
		response, err = client.Do(request)
		if err != nil {
			return err
		}
	}
	drainAndCloseBody(response.Body)
	return nil
}

func replacedResponseOnlyReadByHelper(client *http.Client, request *http.Request, retry bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	if retry {
		drainAndCloseBody(response.Body)
		response, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
		if err != nil {
			return err
		}
	}
	maybeCloseBody(response.Body, false)
	return nil
}

func unrelatedResponseClosedByHelper(client *http.Client, request *http.Request, other *http.Response) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	drainAndCloseBody(other.Body)
	_ = response.StatusCode
	return nil
}

func alternateFileDeferredCleanup(path string, create bool) error {
	var file *os.File
	var err error
	if create {
		file, err = os.Create(path)
		if err != nil {
			return err
		}
	} else {
		file, err = os.Open(path)
		if err != nil {
			return err
		}
	}
	defer func(value *os.File) { _ = value.Close() }(file)
	_, err = file.WriteString("content")
	return err
}
