package resourcelifetime

import (
	"context"
	"errors"
	"net/http"
)

// Context sentinels are documented non-nil errors. Matching the acquisition's
// exact error excludes a live response on that branch; a joined or unrelated
// error, or a target which may be nil, does not supply that evidence.

func closeResponseAfterDeadline(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func closeResponseAfterCancellation(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if errors.Is(err, context.Canceled) {
		return err
	}
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func unrelatedContextError(client *http.Client, request *http.Request, other error) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released"
	if errors.Is(other, context.Canceled) {
		return other
	}
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func joinedContextError(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released"
	if errors.Is(errors.Join(err, context.Canceled), context.Canceled) {
		return err
	}
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func possiblyNilErrorTarget(client *http.Client, request *http.Request, target error) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released"
	if errors.Is(err, target) {
		return err
	}
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}
