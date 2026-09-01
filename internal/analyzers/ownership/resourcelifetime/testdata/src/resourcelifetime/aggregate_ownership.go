package resourcelifetime

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
