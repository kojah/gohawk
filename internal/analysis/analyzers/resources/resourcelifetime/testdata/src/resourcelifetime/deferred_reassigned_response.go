package resourcelifetime

import "net/http"

// A deferred literal closes the current response cell. Reassignments prevent
// exact completion, so its cleanup is unknown rather than proof of a leak.
// This boundary can miss an overwritten response that was never closed.
func deferredReassignedResponse(client *http.Client, request *http.Request) {
	response, err := client.Do(request)
	if err != nil {
		return
	}
	if response.ContentLength == 0 {
		response.Body.Close()
		response, err = client.Do(request)
		if err != nil {
			return
		}
	}
	defer func() { response.Body.Close() }()
}

func deferredDifferentResponse(client *http.Client, request *http.Request, other *http.Response) {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return
	}
	defer func() { other.Body.Close() }()
	_ = response.StatusCode
}

func conditionalDeferredResponse(client *http.Client, request *http.Request, cleanup bool) {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return
	}
	if cleanup {
		defer func() { response.Body.Close() }()
	}
}

func deferredReassignedResponseRead(client *http.Client, request *http.Request) {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return
	}
	if response.ContentLength == 0 {
		response.Body.Close()
		response, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
		if err != nil {
			return
		}
	}
	defer func() { _ = response.StatusCode }()
}
