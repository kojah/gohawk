package resourcelifetime

// Replacing the captured response cell is already an accepted coverage loss in
// ordinary ownership classification; it is not a diagnostic control here.

import (
	"io"
	"net/http"
)

func capturedBodyCleanupOnBothBranches(url string) {
	response, err := http.Get(url)
	if err != nil {
		return
	}
	closeResponse := func() {
		if response.Body != nil {
			_ = response.Body.Close()
		}
	}
	if response.StatusCode == http.StatusOK {
		closeResponse()
		return
	}
	closeResponse()
}

func capturedBodyCleanupCalledImmediately(url string) {
	response, err := http.Get(url)
	if err != nil {
		return
	}
	func() {
		if response.Body != nil {
			_ = response.Body.Close()
		}
	}()
}

func capturedBodyCleanupExtraCondition(url string, closeIt bool) {
	response, err := http.Get(url) // want "owned resource from http.Get is not released"
	if err != nil {
		return
	}
	func() {
		if closeIt && response.Body != nil {
			_ = response.Body.Close()
		}
	}()
}

func capturedBodyCleanupDifferentResponse(url string, other *http.Response) {
	response, err := http.Get(url) // want "owned resource from http.Get is not released"
	if err != nil {
		return
	}
	func() {
		if other.Body != nil {
			_ = other.Body.Close()
		}
	}()
	_ = response
}

func capturedBodyCleanupReplacedField(url string, other io.ReadCloser) {
	response, err := http.Get(url) // want "owned resource from http.Get is not released"
	if err != nil {
		return
	}
	func() {
		response.Body = other
		if response.Body != nil {
			_ = response.Body.Close()
		}
	}()
}

func replaceCapturedBody(response *http.Response, other io.ReadCloser) {
	response.Body = other
}

func capturedBodyCleanupIndirectReplacement(url string, other io.ReadCloser) {
	response, err := http.Get(url) // want "owned resource from http.Get is not released"
	if err != nil {
		return
	}
	func() {
		replaceCapturedBody(response, other)
		if response.Body != nil {
			_ = response.Body.Close()
		}
	}()
}
