package resourcelifetime

// A resource kept behind an interface and released under a comma-ok
// assertion of its own type is released: the assertion cannot fail for that
// value, so the false arm has nothing to release. An assertion of a type the
// resource does not satisfy leaves the release conditional.

import (
	"io"
	"os"
)

type namer interface{ Name() string }

func closedUnderMatchingAssertion(path string) error {
	var reader io.Reader
	reader, err := os.Open(path)
	if err != nil {
		return err
	}
	if file, ok := reader.(*os.File); ok {
		defer file.Close()
	}
	return nil
}

func closedUnderInterfaceAssertion(path string) error {
	var reader io.Reader
	reader, err := os.Open(path)
	if err != nil {
		return err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}
	return nil
}

type readerCloser interface {
	io.Reader
	Close() error
	Truncate(int64) error
	Chdir() error
	Readdirnames(int) ([]string, error)
	SyscallConn() (any, error)
}

func closedUnderUnsatisfiedAssertion(path string) error {
	var reader io.Reader
	reader, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	if special, ok := reader.(readerCloser); ok {
		defer special.Close()
	}
	return nil
}
