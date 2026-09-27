package resourcelifetime

import (
	"os"

	"callbackhelpers"
)

// A cleanup method value handed to a helper that forwards it to a sibling
// which calls it on every return is the release.

// Accepted: the helper forwards file.Close to a sibling that calls it.
func deferredForwardedClose(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer callbackhelpers.LogOnError(file.Close)
	return nil
}

// The helper returns without calling the callback when quiet is set.
func deferredMaybeForwardedClose(path string, quiet bool) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return err
	}
	defer callbackhelpers.MaybeLogOnError(quiet, file.Close)
	return nil
}
