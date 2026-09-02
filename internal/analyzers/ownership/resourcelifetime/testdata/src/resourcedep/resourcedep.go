package resourcedep

import (
	"io"
	"net/http"
	"os"
)

func Close(file *os.File) error { return file.Close() }

func MaybeClose(file *os.File, enabled bool) error {
	if enabled {
		return file.Close()
	}
	return nil
}

func CloseResponse(response *http.Response) {
	defer func() { _ = response.Body.Close() }()
}

func MaybeCloseResponse(response *http.Response, enabled bool) {
	defer func() {
		if enabled {
			_ = response.Body.Close()
		}
	}()
}

func CloseBody(body io.ReadCloser) {
	_ = body.Close()
}

func MaybeCloseBody(body io.ReadCloser, enabled bool) {
	if enabled {
		_ = body.Close()
	}
}

// IgnoreErrorFunc invokes cleanup and discards its error.
func IgnoreErrorFunc(cleanup func() error) {
	_ = cleanup()
}

// MaybeIgnoreErrorFunc invokes cleanup only when enabled.
func MaybeIgnoreErrorFunc(cleanup func() error, enabled bool) {
	if enabled {
		_ = cleanup()
	}
}
