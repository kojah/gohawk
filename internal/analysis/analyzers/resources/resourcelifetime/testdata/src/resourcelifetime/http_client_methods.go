package resourcelifetime

// Client.Get, Client.Post, and Client.PostForm carry the documented body
// obligation of the package functions they generalize. Client.Head does not,
// and a type that merely shares the name Client is not net/http's.

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

func clientGetLeaksOnStatus(client *http.Client, target string) (int, error) {
	response, err := client.Get(target) // want "owned resource from http.Get is not released on every return path"
	if err != nil {
		return 0, err
	}
	if response.StatusCode >= 500 {
		return 0, errors.New("server error")
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}

func clientPostLeaks(client *http.Client, target string) error {
	response, err := client.Post(target, "text/plain", strings.NewReader("body")) // want "owned resource from http.Post is not released"
	if err != nil {
		return err
	}
	_ = response.StatusCode
	return nil
}

func clientPostFormLeaks(client *http.Client, target string) error {
	response, err := client.PostForm(target, url.Values{"key": {"value"}}) // want "owned resource from http.PostForm is not released"
	if err != nil {
		return err
	}
	_ = response.StatusCode
	return nil
}

func clientGetClosed(client *http.Client, target string) (int, error) {
	response, err := client.Get(target)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}

func clientGetReturnedToCaller(client *http.Client, target string) (*http.Response, error) {
	return client.Get(target)
}

func clientHeadUnclosed(client *http.Client, target string) (int, error) {
	response, err := client.Head(target)
	if err != nil {
		return 0, err
	}
	return response.StatusCode, nil
}

// Client is a project type whose Get method returns a response without
// promising anything about who closes it.
type Client struct{ inner *http.Client }

func (c *Client) Get(target string) (*http.Response, error) { return nil, nil }

func projectClientGetUnclosed(c *Client, target string) (int, error) {
	response, err := c.Get(target)
	if err != nil {
		return 0, err
	}
	return response.StatusCode, nil
}
