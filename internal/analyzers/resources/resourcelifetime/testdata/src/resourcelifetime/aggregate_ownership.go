package resourcelifetime

// Accepted gaps: a resource stored into a local map, appended to a local
// slice, or appended through a local pointer is an opaque consumption. The
// analysis cannot see whether that storage is drained later, so no diagnostic
// is claimed even when the storage is never used again, or when a deferred
// drain releases a different local slice.

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"time"
)

type compositeReadCloser struct {
	io.Reader
	closers []func() error
}

func (c *compositeReadCloser) Close() error {
	for _, closeResource := range c.closers {
		_ = closeResource()
	}
	return nil
}

func transferredGzipReader(source io.Reader) (io.ReadCloser, error) {
	reader, err := gzip.NewReader(source)
	if err != nil {
		return nil, err
	}
	return &compositeReadCloser{Reader: reader, closers: []func() error{reader.Close}}, nil
}

type fileSource interface {
	Close() error
}

type realFileSource struct {
	file *os.File
}

func (source *realFileSource) Close() error {
	return source.file.Close()
}

func settleSource(source fileSource) {
	defer source.Close()
}

func aggregateClosedByHelper() error {
	file, err := os.Open("state")
	if err != nil {
		return err
	}
	settleSource(&realFileSource{file: file})
	return nil
}

type closerOwner struct {
	closers []io.Closer
}

type outerCloserOwner struct {
	inner *closerOwner
}

func transferredNestedOwner(target *outerCloserOwner) error {
	file, err := os.Open("state")
	if err != nil {
		return err
	}
	target.inner = &closerOwner{closers: []io.Closer{file}}
	return nil
}

func returnedNestedOwner() (*closerOwner, error) {
	file, err := os.Open("state")
	if err != nil {
		return nil, err
	}
	return &closerOwner{closers: []io.Closer{file}}, nil
}

func returnedAppendedOwner(extra bool) (*closerOwner, error) {
	file, err := os.Open("state")
	if err != nil {
		return nil, err
	}
	closers := []io.Closer{file}
	if extra {
		closers = append(closers, io.NopCloser(bytes.NewReader(nil)))
	}
	return &closerOwner{closers: closers}, nil
}

type owner struct {
	timer *time.Timer
	file  *os.File
}

func transferredTimer(target *owner) {
	target.timer = time.NewTimer(time.Second)
}

func partiallyConstructedOwner(fail bool) (*owner, error) {
	file, err := os.Open("state") // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return nil, err
	}
	result := &owner{file: file}
	if fail {
		return nil, errors.New("finish initialization")
	}
	return result, nil
}

func cleanedPartialOwner(fail bool) (*owner, error) {
	file, err := os.Open("state")
	if err != nil {
		return nil, err
	}
	result := &owner{file: file}
	if fail {
		_ = file.Close()
		return nil, errors.New("finish initialization")
	}
	return result, nil
}

func returnedOwner() (*owner, error) {
	file, err := os.Open("state")
	if err != nil {
		return nil, err
	}
	return &owner{file: file}, nil
}

type noOpFileOwner struct{}

func (*noOpFileOwner) Close() error { return nil }

func (*noOpFileOwner) Add(*os.File) {}

func (current *noOpFileOwner) With(*os.File) *noOpFileOwner { return current }

func filePassedToNoOpAdd(current *noOpFileOwner) error {
	file, err := os.Open("state") // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	current.Add(file)
	return nil
}

func filePassedToIgnoredNoOpWith(current *noOpFileOwner) error {
	file, err := os.Open("state") // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	current.With(file)
	return nil
}

type fluentFileOwner struct{ file *os.File }

func (owner *fluentFileOwner) WithFile(file *os.File) *fluentFileOwner {
	owner.file = file
	return owner
}

func (owner *fluentFileOwner) Close() error { return owner.file.Close() }

func returnedFluentFileOwner() (*fluentFileOwner, error) {
	file, err := os.Open("fixture")
	if err != nil {
		return nil, err
	}
	owner := (&fluentFileOwner{}).WithFile(file)
	return owner, nil
}

type readCloserWithHook struct {
	closeFn func() error
}

// A struct literal returned by value carries the file whose Close method
// value its callback captured, so the caller owns the file through it.
func returnedLiteralCarriesMethodValue(path string) (readCloserWithHook, error) {
	file, err := os.Open(path)
	if err != nil {
		return readCloserWithHook{}, err
	}
	fileClose := file.Close
	return readCloserWithHook{closeFn: func() error { return fileClose() }}, nil
}

type logFileRecord struct {
	file *os.File
}

type logFileManager struct {
	files map[string]*logFileRecord
}

// Storing a record that holds the file into the receiver's map transfers the
// file to the manager, just as storing the file itself would.
func (manager *logFileManager) openRecord(key, path string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	manager.files[key] = &logFileRecord{file: file}
	return nil
}

type profileSession struct {
	closers []func()
}

func (session *profileSession) appendCloser(closer func()) {
	session.closers = append(session.closers, closer)
}

// A closer that closes the file, appended to the receiver's closer list
// through a source-visible method, transfers the file to the session.
func (session *profileSession) startProfile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	session.appendCloser(func() { _ = file.Close() })
	return nil
}

// Files appended to a local closer slice that a deferred literal drains are
// released by that literal; the deferred release may run zero times only when
// nothing was appended.
func filesAppendedToDeferredCloserSlice(paths []string) (err error) {
	var closers []io.Closer
	defer func() {
		for i := range closers {
			if cerr := closers[i].Close(); cerr != nil && err == nil {
				err = cerr
			}
		}
	}()
	for _, path := range paths {
		file, oerr := os.Open(path)
		if oerr != nil {
			return oerr
		}
		closers = append(closers, file)
	}
	return nil
}

type outputFiles []io.Closer

// Appending the file to the slice a pointer receiver points at stores it in
// caller-owned storage, which owns the release.
func (outputs *outputFiles) Set(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	*outputs = append(*outputs, file)
	return nil
}
