package resourcelifetime

import (
	"io"
	"os"

	"resourcelifetime/wrapping"
)

// A constructor that keeps its argument in the value it returns has not taken
// over releasing it: unless the returned type releases the argument, the
// caller still holds the obligation. The summaries now say so, because the
// search follows the store into the unexported helper that performs it, which
// is how bufio.NewReader reaches its reader through (*bufio.Reader).reset.
//
// Accepted false negative: the leaks below are not reported. ReturnedOwner is
// now correct for these constructors, but returnedViews will not narrow it to
// a view, for two reasons. It requires the parameter's own type to have a
// recognized cleanup method, and a wrapping constructor takes an interface
// such as io.Reader that has none, even though the argument is an *os.File.
// It also asks which field of the returned struct holds the parameter, which
// a delegated store does not reveal. Both must be addressed before a file
// handed to a non-releasing wrapper is reported, so no diagnostic is claimed
// here; the accepted forms below are the part that is proven today.

// fileWrappedByReleasingConstructor is accepted because the wrapper closes the
// file and this function closes the wrapper on every path.
func fileWrappedByReleasingConstructor(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	wrapped := wrapping.NewCloser(file)
	defer func() { _ = wrapped.Close() }()
	return nil
}

// fileWrappedWithoutRelease documents the gap above: the wrapper only reads
// through the file, so the obligation stays here, and nothing releases it.
func fileWrappedWithoutRelease(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	_, err = io.ReadAll(wrapping.NewReader(file))
	return err
}

// fileWrappedThroughFastPath documents the same gap through the constructor
// that returns the argument itself when it already fits.
func fileWrappedThroughFastPath(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	_, err = io.ReadAll(wrapping.NewReaderFastPath(file))
	return err
}
