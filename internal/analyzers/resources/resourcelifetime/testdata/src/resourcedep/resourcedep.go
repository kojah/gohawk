package resourcedep

import (
	"database/sql"
	"errors"
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

// CheckStatus closes an unsuccessful response and returns successful responses
// unchanged, preserving their ownership for the caller.
func CheckStatus(response *http.Response, err error) (*http.Response, error) {
	if err != nil || response.StatusCode == http.StatusOK {
		return response, err
	}
	defer func() { _ = response.Body.Close() }()
	return response, errors.New("unexpected response status")
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

func CloseViaCallback(closer io.Closer) {
	IgnoreErrorFunc(closer.Close)
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

// ValueRegistry models a receiver retaining variadic results for a later caller.
type ValueRegistry struct{ values func() []any }

// Keep retains the values through a callback stored on the receiver.
func (registry *ValueRegistry) Keep(values ...any) *ValueRegistry {
	registry.values = func() []any { return values }
	return registry
}

// Observe only counts the arguments and retains none of them.
func (*ValueRegistry) Observe(values ...any) int { return len(values) }

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

// FileHolder carries a file by value.
type FileHolder struct{ File *os.File }

// CloseHolder closes the file through its by-value copy of the holder.
func CloseHolder(holder FileHolder) error { return holder.File.Close() }

// CloseEach closes every non-nil file it is given, as client-go's
// CloseAndRemove does.
func CloseEach(name string, files ...*os.File) {
	_ = name
	for _, file := range files {
		if file != nil {
			_ = file.Close()
		}
	}
}

// InspectEach reads every file it is given and closes none.
func InspectEach(files ...*os.File) {
	for _, file := range files {
		_ = file
	}
}

// Failed reports whether err is non-nil.
func Failed(err error) bool { return err != nil }

// FailedOther ignores its first error and reports on the second.
func FailedOther(_ error, other error) bool { return other != nil }

// Unchanged returns the file it was given.
func Unchanged(file *os.File) *os.File { return file }

// Replaced returns the other file.
func Replaced(_ *os.File, other *os.File) *os.File { return other }

// Erased returns the file behind an interface.
func Erased(file *os.File) any { return file }

var registry []*os.File

// OpenFresh opens and returns a file the caller must close.
func OpenFresh(path string) (*os.File, error) { return os.Open(path) }

// OpenPair hands two separately owned files to the caller on success.
func OpenPair(path string) (*os.File, *os.File, error) {
	first, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	second, err := os.Open(path)
	if err != nil {
		_ = first.Close()
		return nil, nil, err
	}
	return first, second, nil
}

// OpenReader returns the opened file behind a closer interface.
func OpenReader(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// OpenClosedOnFailure closes the file only on its failure path.
func OpenClosedOnFailure(path string, ok bool) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		_ = f.Close()
		return nil, errors.New("rejected")
	}
	return f, nil
}

// OpenRegistered keeps the file in a package registry as well.
func OpenRegistered(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	registry = append(registry, f)
	return f, nil
}

// OpenSeeked positions the file before handing it back.
func OpenSeeked(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	_, _ = f.Seek(0, 0)
	return f, nil
}

// OpenWithCleanup returns the file and a callback that closes it.
func OpenWithCleanup(path string) (*os.File, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

// OpenMaybeClosed may close the file and still return it.
func OpenMaybeClosed(path string, flush bool) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if flush {
		_ = f.Close()
	}
	return f, nil
}

// OpenView hands the caller's own file back.
func OpenView(file *os.File) *os.File { return file }

// Pair holds two files by value.
type Pair struct {
	First  *os.File
	Second *os.File
	Name   string
}

var (
	keptFile  *os.File
	keptNames []string
	fileSink  interface{ Add(any) }
)

// KeepFirst stores the first file of the pair beyond the call.
func KeepFirst(pair *Pair) { keptFile = pair.First }

// KeepCopy stores a file loaded from a whole copy of the pair.
func KeepCopy(pair *Pair) { copied := *pair; keptFile = copied.Second }

// PublishFirst hands the first file to an interface it cannot see through.
func PublishFirst(pair *Pair) { fileSink.Add(pair.First) }

// StartWith reads the pair on another goroutine.
func StartWith(pair *Pair) { go InspectPair(pair) }

// FirstOf returns the first file of the pair.
func FirstOf(pair *Pair) *os.File { return pair.First }

// KeepName stores only the pair's name, which holds no resource.
func KeepName(pair *Pair) { keptNames = append(keptNames, pair.Name) }

// InspectPair only reads the pair.
func InspectPair(pair *Pair) bool { return pair.First != nil }

// CloseFirst closes only the first file of the pair.
func CloseFirst(pair *Pair) error { return pair.First.Close() }

// CloseSecond closes only the second file of the pair.
func CloseSecond(pair *Pair) error { return pair.Second.Close() }

// CloseHead closes only the first element.
func CloseHead(files [2]*os.File) error { return files[0].Close() }

// CloseTail closes only the second element.
func CloseTail(files [2]*os.File) error { return files[1].Close() }

// ReadFirst reads from the file on every path, so a caller that has closed
// the file uses it after Close by calling this helper.
func ReadFirst(file *os.File) error {
	_, err := file.Read(make([]byte, 1))
	return err
}

// ReadSometimes reads only when asked, so it requires nothing of the file.
func ReadSometimes(file *os.File, enabled bool) error {
	if !enabled {
		return nil
	}
	_, err := file.Read(make([]byte, 1))
	return err
}

// LastErr calls Err, which the rows contract allows after Close.
func LastErr(rows *sql.Rows) error { return rows.Err() }

// ReadReplaced reads a file of its own, not the one it was handed.
func ReadReplaced(file *os.File, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Read(make([]byte, 1))
	return err
}

// ScanAll iterates the rows on every path: closed rows silently yield none.
func ScanAll(rows *sql.Rows) error {
	for rows.Next() {
		var value int
		if err := rows.Scan(&value); err != nil {
			return err
		}
	}
	return rows.Err()
}
