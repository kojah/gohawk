package resourceforward

import (
	"io"
	"resourcedep"
)

func CleanupFor(resource io.Closer) func() {
	return resourcedep.CleanupFor(resource)
}

func CaptureError(errp *error, fn func() error) func() {
	return func() {
		err := fn()
		if *errp == nil {
			*errp = err
		}
	}
}

func IgnoreError(_ *error, _ func() error) func() { return func() {} }

func DiscardCallback(_ func() error) func() error { return func() error { return nil } }
