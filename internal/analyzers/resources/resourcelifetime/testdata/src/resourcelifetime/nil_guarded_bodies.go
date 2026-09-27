package resourcelifetime

import (
	"io"
	"net/http"
)

// A close guarded by the response and its Body being non-nil covers every
// feasible path: a response with a nil Body holds nothing to close.

// Accepted: the Authula shape, a defer registered under both nil checks.
func deferUnderResponseAndBodyGuard(client *http.Client, req *http.Request) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	return io.ReadAll(resp.Body)
}

// Accepted: the Body check alone.
func deferUnderBodyGuard(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	return nil
}

// Accepted: the negated disjunction returns before the close on the nil arms.
func returnUnlessResponseAndBody(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	if resp == nil || resp.Body == nil {
		return nil
	}
	defer resp.Body.Close()
	return nil
}

// A guard that also requires an unrelated flag leaves the body open when the
// flag is false.
func deferUnderBodyAndFlag(url string, keep bool) error {
	resp, err := http.Get(url) // want "owned resource from http.Get is not released"
	if err != nil {
		return err
	}
	if resp.Body != nil && !keep {
		defer resp.Body.Close()
	}
	return nil
}

// Another response's Body says nothing about this one.
func deferUnderOtherBodyGuard(url string, other *http.Response) error {
	resp, err := http.Get(url) // want "owned resource from http.Get is not released"
	if err != nil {
		return err
	}
	if other.Body != nil {
		defer resp.Body.Close()
	}
	return nil
}
