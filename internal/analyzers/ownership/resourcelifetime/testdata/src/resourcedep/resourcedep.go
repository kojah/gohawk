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

// Journal owns the file its constructor opened; Close releases it.
type Journal struct {
	file *os.File
	name string
}

// OpenJournal acquires the file and hands ownership to the caller.
func OpenJournal(path string) (*Journal, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Journal{file: file, name: path}, nil
}

// Append writes without releasing anything.
func (j *Journal) Append(line string) error {
	_, err := j.file.WriteString(line)
	return err
}

// Close releases the file on every return.
func (j *Journal) Close() error { return j.file.Close() }

// View wraps a caller's file without owning it.
type View struct {
	file *os.File
}

// NewView stores the caller's file, so the caller still owns it.
func NewView(file *os.File) *View { return &View{file: file} }

// Close of a view does not release the caller's file.
func (v *View) Close() error { return nil }

// Sink owns a file but no method releases it.
type Sink struct {
	file *os.File
}

// OpenSink acquires the file but its type offers no release.
func OpenSink(path string) (*Sink, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &Sink{file: file}, nil
}

// Flush syncs without closing.
func (s *Sink) Flush() error { return s.file.Sync() }

// AdoptJournal stores the caller's file in a journal, whose Close releases it.
func AdoptJournal(file *os.File) *Journal { return &Journal{file: file} }

// DrainResponse drains and closes the body of a response that has one.
func DrainResponse(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
