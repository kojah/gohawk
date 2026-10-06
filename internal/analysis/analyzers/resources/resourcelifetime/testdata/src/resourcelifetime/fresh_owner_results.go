package resourcelifetime

// A helper that acquires a resource and hands it straight back gives the
// caller the obligation: the helper's summary names the result as owned,
// and the result type names the cleanup. The claim is strict about
// freshness. A helper that also registers the resource, touches it, may
// have closed it, captures it in a returned callback, or returns the
// caller's own value makes no claim, so those callers are not reported even
// when they drop the value; the last two are accepted false negatives.

import (
	"os"

	"resourcedep"
)

// OpenLocalFresh is an exported same-package helper with the same contract.
func OpenLocalFresh(path string) (*os.File, error) { return os.Open(path) }

// OpenForwarded forwards an imported helper's owned result, so its own
// summary claims the result too.
func OpenForwarded(path string) (*os.File, error) { return resourcedep.OpenFresh(path) }

func openPrivately(path string) (*os.File, error) { return os.Open(path) }

// OpenThroughPrivateHelper forwards a private helper's result; with no
// summary for the helper, nothing proves the acquisition.
func OpenThroughPrivateHelper(path string) (*os.File, error) { return openPrivately(path) }

func dropsFreshResult(path string) error {
	file, err := resourcedep.OpenFresh(path) // want "owned resource from resourcedep.OpenFresh is not released on every return path"
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func dropsFreshReader(path string) error {
	reader, err := resourcedep.OpenReader(path) // want "owned resource from resourcedep.OpenReader is not released on every return path"
	if err != nil {
		return err
	}
	_ = reader
	return nil
}

func dropsResultClosedOnlyOnFailure(path string) error {
	file, err := resourcedep.OpenClosedOnFailure(path, true) // want "owned resource from resourcedep.OpenClosedOnFailure is not released on every return path"
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func dropsLocalFreshResult(path string) error {
	file, err := OpenLocalFresh(path) // want "owned resource from resourcelifetime.OpenLocalFresh is not released on every return path"
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func closesFreshResult(path string) error {
	file, err := resourcedep.OpenFresh(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

func returnsFreshResult(path string) (*os.File, error) {
	return resourcedep.OpenFresh(path)
}

func dropsRegisteredResult(path string) error {
	file, err := resourcedep.OpenRegistered(path)
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func dropsSeekedResult(path string) error {
	file, err := resourcedep.OpenSeeked(path)
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func usesReturnedCleanup(path string) error {
	file, cleanup, err := resourcedep.OpenWithCleanup(path)
	if err != nil {
		return err
	}
	defer cleanup()
	_ = file
	return nil
}

func dropsMaybeClosedResult(path string) error {
	file, err := resourcedep.OpenMaybeClosed(path, true)
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func dropsViewedResult(file *os.File) {
	_ = resourcedep.OpenView(file)
}

func dropsForwardedResult(path string) error {
	file, err := OpenForwarded(path) // want "owned resource from resourcelifetime.OpenForwarded is not released on every return path"
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func dropsPrivatelyForwardedResult(path string) error {
	file, err := OpenThroughPrivateHelper(path)
	if err != nil {
		return err
	}
	_ = file
	return nil
}
