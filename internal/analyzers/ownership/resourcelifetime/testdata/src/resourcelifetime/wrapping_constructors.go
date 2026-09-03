package resourcelifetime

import (
	"io"
	"os"

	"resourcelifetime/wrapping"
	"resourcelifetime/wrapping/outer"
)

// A constructor that keeps its argument in the value it returns has not taken
// over releasing it: unless the returned type can release the argument, the
// caller still holds the obligation. Proving that across a package boundary
// depends on the constructor's summary, and a constructor commonly delegates
// the store to an unexported helper that is never summarized, so the search
// follows the call into that helper's body. bufio.NewReader reaches its reader
// through (*bufio.Reader).reset this way, and was summarized as keeping the
// reader for itself until it did.
//
// Whether the wrapper releases the argument is decided by the returned type,
// not by a mask that came back empty: a type carrying no cleanup method offers
// no way to release anything it holds, while a type that does carry one may
// release this field, and the proof stops rather than guess.

// fileWrappedWithoutRelease hands the file to a wrapper that only reads
// through it. Nothing on that wrapper can close the file, so the obligation
// stays here and the file leaks.
func fileWrappedWithoutRelease(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	_, err = io.ReadAll(wrapping.NewReader(file))
	return err
}

// fileWrappedThroughFastPath leaks for the same reason through the constructor
// that returns the argument itself when it already fits.
func fileWrappedThroughFastPath(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	_, err = io.ReadAll(wrapping.NewReaderFastPath(file))
	return err
}

// fileWrappedByReleasingConstructor is accepted: the wrapper carries a Close,
// so it may release the file, and this function closes the wrapper on every
// path.
func fileWrappedByReleasingConstructor(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	wrapped := wrapping.NewCloser(file)
	defer func() { _ = wrapped.Close() }()
	return nil
}

// fileWrappedByReleasingConstructorWithoutClose is accepted for a different
// reason: the wrapper may release the file, so this function is not proven to
// be the one that must, and an unproven transfer suppresses rather than
// reports.
func fileWrappedByReleasingConstructorWithoutClose(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	_ = wrapping.NewCloser(file)
	return nil
}

// fileWrappedAcrossTwoPackages hands the file to a constructor whose own store
// happens in a third package. The summary carries that across the boundary, so
// the file is still this function's to close and leaks.
func fileWrappedAcrossTwoPackages(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	_, err = io.ReadAll(outer.NewOuter(file))
	return err
}

// fileWrappedAcrossTwoPackagesBySealed reports even though the outer type
// carries a Close. It was handed an io.Reader, which offers no way to close
// the file, so that Close cannot be the one that releases it.
func fileWrappedAcrossTwoPackagesBySealed(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	_ = outer.NewSealed(file)
	return nil
}
