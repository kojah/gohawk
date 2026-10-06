package resourcedep

import "io"

// CloseQuietly releases whatever it is given through the io.Closer interface.
func CloseQuietly(closer io.Closer) {
	if err := closer.Close(); err != nil {
		println(err.Error())
	}
}

// RecordClose hands the closer's bound Close method to another helper that
// calls it.
func RecordClose(slot *error, closer io.Closer) { recordError(slot, closer.Close) }

func recordError(slot *error, closeFn func() error) {
	if err := closeFn(); err != nil && *slot == nil {
		*slot = err
	}
}

// Ignore accepts a closer and never closes it.
func Ignore(closer io.Closer) {}
