package resourcelifetime

import (
	"bytes"
	"io"
	"net/http"
)

func leakedResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	_ = response
	return nil
}

func closedResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func responseBodyClosedThroughDeferredParameter(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func(body io.ReadCloser) {
		_ = body.Close()
	}(response.Body)
	return nil
}

func deferredParameterClosesDifferentResponse(client *http.Client, first, second *http.Request) error {
	leaked, err := client.Do(first) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	closed, err := client.Do(second)
	if err != nil {
		_ = leaked.Body.Close()
		return err
	}
	defer func(body io.ReadCloser) {
		_ = body.Close()
	}(closed.Body)
	_ = leaked
	return nil
}

func deferredParameterClosesDifferentParameter(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	defer func(unrelated, body io.ReadCloser) {
		_ = unrelated.Close()
	}(io.NopCloser(bytes.NewReader(nil)), response.Body)
	return nil
}

func deferredParameterConditionallyCloses(client *http.Client, request *http.Request, enabled bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	defer func(body io.ReadCloser) {
		if enabled {
			_ = body.Close()
		}
	}(response.Body)
	return nil
}

func immediateParameterCloseDoesNotSettleReturn(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	func(body io.ReadCloser) {
		_ = body.Close()
	}(response.Body)
	return nil
}

func invokeCleanup(cleanup func() error) { _ = cleanup() }

func maybeInvokeCleanup(cleanup func() error, enabled bool) {
	if enabled {
		_ = cleanup()
	}
}

func observeCleanup(func() error) {}

func responseClosedByDeferredHelper(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer invokeCleanup(response.Body.Close)
	return nil
}

func conditionalDeferredHelperLeaksResponse(client *http.Client, request *http.Request, enabled bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	defer maybeInvokeCleanup(response.Body.Close, enabled)
	return nil
}

func nonDeferredObserverLeaksResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	observeCleanup(response.Body.Close)
	return nil
}

func deferredHelperClosesOnlyItsBoundResponse(client *http.Client, first, second *http.Request) error {
	closed, err := client.Do(first)
	if err != nil {
		return err
	}
	leaked, err := client.Do(second) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		_ = closed.Body.Close()
		return err
	}
	defer invokeCleanup(closed.Body.Close)
	_ = leaked
	return nil
}

func responseClosedByImmediateNestedDefer(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	func() {
		defer func() { _ = response.Body.Close() }()
	}()
	return nil
}

func responseConditionallyClosedByImmediateNestedDefer(client *http.Client, request *http.Request, closeBody bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	func() {
		if closeBody {
			defer func() { _ = response.Body.Close() }()
		}
	}()
	return nil
}

func responseCleanupClosureNotCalled(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	cleanup := func() { _ = response.Body.Close() }
	_ = cleanup
	return nil
}

func returnedResponseBody(client *http.Client, request *http.Request) (io.ReadCloser, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}

func responseOwnedByWorker(client *http.Client, request *http.Request) (<-chan struct{}, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer response.Body.Close()
	}()
	return done, nil
}

func conditionallyReturnedResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	return err
}
