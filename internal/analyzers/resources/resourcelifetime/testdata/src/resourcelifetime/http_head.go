package resourcelifetime

import (
	"net/http"
	"time"
)

func headZeroClient(url string) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	_, err = client.Do(request)
	return err
}

func getZeroClient(url string) error {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	_, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}

func headTimeout(url string) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: time.Second}
	_, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}

func headCustomTransport(url string, transport http.RoundTripper) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Transport: transport}
	_, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}

func headMethodChanged(url string) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	request.Method = "GET"
	client := &http.Client{}
	_, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}
