package resourcelifetime

import (
	"os"
	"resourceforward"
)

// The returned callback invokes the exact bound Close on every invocation.
func deferredBoundCleanup(path string) (retErr error) {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer resourceforward.CaptureError(&retErr, file.Close)()
	return nil
}

func deferredWrongBoundCleanup(path string, other *os.File) (retErr error) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return err
	}
	defer resourceforward.CaptureError(&retErr, other.Close)()
	_ = file.Name()
	return nil
}

func deferredIgnoredBoundCleanup(path string) (retErr error) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return err
	}
	defer resourceforward.IgnoreError(&retErr, file.Close)()
	return nil
}

func deferredDiscardedBoundCleanup(path string) (retErr error) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return err
	}
	defer resourceforward.CaptureError(&retErr, resourceforward.DiscardCallback(file.Close))()
	return nil
}
