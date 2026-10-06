package resourcelifetime

import (
	"net/http"
	"os"
)

// Captured cleanup registered before acquisition observes the cell at return,
// not its initial nil contents. This is possible cleanup, not a guarantee:
// later overwrites of that cell can hide real leaks under this boundary.
func responseCapturedBeforeAcquisition(url string, get bool) error {
	var response *http.Response
	defer func() {
		if response != nil {
			response.Body.Close()
		}
	}()
	var err error
	if get {
		response, err = http.Get(url)
	} else {
		response, err = http.Post(url, "text/plain", nil)
	}
	return err
}

func unrelatedPriorCapturedCleanup(url string, other *http.Response) error {
	defer func() {
		if other != nil {
			other.Body.Close()
		}
	}()
	response, err := http.Get(url) // want "owned resource from http.Get is not released"
	_ = response
	return err
}

func readOnlyPriorCapture(url string) error {
	var response *http.Response
	defer func() {
		if response != nil {
			_ = response.StatusCode
		}
	}()
	var err error
	response, err = http.Get(url) // want "owned resource from http.Get is not released"
	return err
}

func conditionalPriorCapture(url string, cleanup bool) error {
	var response *http.Response
	if cleanup {
		defer func() {
			if response != nil {
				response.Body.Close()
			}
		}()
	}
	var err error
	response, err = http.Get(url) // want "owned resource from http.Get is not released"
	return err
}

func priorValueArgumentIsNotACapture(url string) error {
	var response *http.Response
	defer func(saved *http.Response) {
		if saved != nil {
			saved.Body.Close()
		}
	}(response)
	var err error
	response, err = http.Get(url) // want "owned resource from http.Get is not released"
	return err
}

func priorCleanupOfNewFileDoesNotCleanCapturedFile(path string, first bool) error {
	var file *os.File
	defer func() {
		if file != nil {
			other, err := os.Open(file.Name())
			if err == nil {
				other.Close()
			}
		}
	}()
	var err error
	if first {
		file, err = os.Open(path) // want "owned resource from os.Open is not released"
	} else {
		file, err = os.Open(path) // want "owned resource from os.Open is not released"
	}
	return err
}
