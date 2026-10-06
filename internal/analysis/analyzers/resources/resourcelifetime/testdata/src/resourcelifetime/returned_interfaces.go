package resourcelifetime

import (
	"io"
	"net/http"
)

func returnNarrowedBody(url string) (io.Reader, error) {
	response, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}

func returnDifferentBody(url string, other io.ReadCloser) (io.Reader, error) {
	response, err := http.Get(url) // want "owned resource from http.Get is not released"
	if err != nil {
		return nil, err
	}
	_ = response
	return other, nil
}

func returnNarrowedBodyOnlySometimes(url string, abandon bool) (io.Reader, error) {
	response, err := http.Get(url) // want "owned resource from http.Get is not released"
	if err != nil {
		return nil, err
	}
	if abandon {
		return nil, nil
	}
	return response.Body, nil
}
