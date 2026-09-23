package resourcelifetime

import "net/http"

// A helper receiving the exact acquisition and error can correlate cleanup
// with success. Until conditional summaries express that relation, a possible
// cleanup makes the call unknown. This deliberately misses helpers that take
// that pair but condition cleanup on another value as well, and helpers that
// take an error the caller tests but condition cleanup on a flag instead.
func consumeResponsePair(response *http.Response, err error) {
	if err != nil {
		return
	}
	defer response.Body.Close()
}

func responsePairTransferred(client *http.Client, request *http.Request) {
	consumeResponsePair(client.Do(request))
}

func consumeGenericResponsePair[T any](response *http.Response, err error) T {
	var result T
	if err != nil {
		return result
	}
	defer response.Body.Close()
	return result
}

func genericResponsePairTransferred(client *http.Client, request *http.Request) int {
	return consumeGenericResponsePair[int](client.Do(request))
}

func unrelatedErrorDoesNotTransferPair(client *http.Client, request *http.Request, other error) {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return
	}
	consumeResponsePair(response, other)
}

func inspectResponsePair(response *http.Response, err error) int {
	if err != nil {
		return 0
	}
	return response.StatusCode
}

func readOnlyPairDoesNotTransfer(client *http.Client, request *http.Request) int {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	return inspectResponsePair(response, err)
}

func conditionalResponseCleanup(response *http.Response, closeIt bool) {
	if closeIt {
		response.Body.Close()
	}
}

func conditionalCleanupWithoutPair(client *http.Client, request *http.Request, closeIt bool) {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return
	}
	conditionalResponseCleanup(response, closeIt)
}

func closeResponseOnError(response *http.Response, err error) {
	if err != nil {
		response.Body.Close()
	}
}

func validateResponse(response *http.Response) error {
	if response.StatusCode != http.StatusOK {
		return http.ErrNotSupported
	}
	return nil
}

// The helper closes on exactly the error the caller then branches on, so
// the path that skips the close is the path that returns the response.
func callerTestedErrorHelperCleanup(client *http.Client, request *http.Request) (*http.Response, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	err = validateResponse(response)
	closeResponseOnError(response, err)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// Testing the error before the helper runs does not correlate the helper's
// cleanup with any later path: when the error is nil, nothing closes it.
func errorTestedOnlyBeforeHelper(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	err = validateResponse(response)
	if err != nil {
		return err
	}
	closeResponseOnError(response, err)
	return nil
}
