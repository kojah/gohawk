package resourcedep

import (
	"database/sql"
	"io"
	"net/http"
	"os"
)

// FinishTransaction rolls the transaction back on every return.
func FinishTransaction(tx *sql.Tx) { _ = tx.Rollback() }

// MaybeFinishTransaction rolls back only when enabled.
func MaybeFinishTransaction(tx *sql.Tx, enabled bool) {
	if enabled {
		_ = tx.Rollback()
	}
}

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

var exitHandlers []func()

// RegisterExit keeps the handler to run at exit.
func RegisterExit(handler func()) {
	exitHandlers = append(exitHandlers, handler)
}
