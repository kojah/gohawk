package resourcelifetime

import (
	"errors"
	"os"
)

// Deferred cleanups guarded by a named result. A deferred literal that closes
// only while the function's own error result is non-nil runs at return time,
// after the return statement has set that result. So each return is judged by
// the value it returns: a nil literal skips the cleanup, a value that is never
// nil runs it, and anything else leaves the resource unknown. A guard on any
// other variable keeps the existing data-dependent policy.

func validateFile(file *os.File) error {
	if file.Name() == "" {
		return errors.New("unnamed file")
	}
	return nil
}

// Closed on every error return, handed to the caller on success.
func closedOnErrorReturnedOnSuccess(path string) (file *os.File, err error) {
	file, err = os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			file.Close()
		}
	}()
	if err = validateFile(file); err != nil {
		return nil, err
	}
	return file, nil
}

// The success return keeps the file open and hands it to nobody.
func closedOnlyOnError(path string) (err error) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			file.Close()
		}
	}()
	if err = validateFile(file); err != nil {
		return err
	}
	return nil
}

// Whether validateFile returns nil is not known, so neither is whether the
// cleanup runs.
func closedOnErrorOfUnknownResult(path string) (err error) {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			file.Close()
		}
	}()
	return validateFile(file)
}

// A guard that closes on success runs on the nil return.
func closedOnSuccess(path string) (err error) {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			file.Close()
		}
	}()
	return nil
}

// A non-nil error value makes the guarded cleanup run.
func closedOnFreshError(path string) (err error) {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			file.Close()
		}
	}()
	return errors.New("rejected")
}

// A guard on a local flag, not a result, keeps the data-dependent policy.
func closedUnlessKept(path string, keep bool) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	kept := false
	defer func() {
		if !kept {
			file.Close()
		}
	}()
	if keep {
		kept = true
	}
	return nil
}

// A shadowed err inside the literal is not the named result.
func shadowedErrorGuard(path string) (err error) {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := validateFile(file); err != nil {
			file.Close()
		}
	}()
	return nil
}
