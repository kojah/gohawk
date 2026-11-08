package httpdemo

import (
	"fmt"
	"io"
	"net/http"
)

// Fetch downloads a successful HTTP response.
func Fetch(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	return body, err
}
