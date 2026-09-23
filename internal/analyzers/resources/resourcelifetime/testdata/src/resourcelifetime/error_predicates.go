package resourcelifetime

import (
	"errors"
	"fmt"
	"os"

	"resourcedep"
)

var observedErrors chan error

func observeOpenError(err error) bool {
	if err != nil {
		select {
		case observedErrors <- err:
		default:
		}
		return true
	}
	return false
}

func closeAfterObservedError(path string) {
	file, err := os.Open(path)
	if observeOpenError(err) {
		return
	}
	defer file.Close()
}

func checkDifferentError(_ error, other error) bool {
	if other != nil {
		return true
	}
	return false
}

func unrelatedErrorPredicate(path string, other error) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if checkDifferentError(err, other) {
		return
	}
	defer file.Close()
}

func wrapBeforeChecking(err error) bool {
	err = fmt.Errorf("wrapped: %v", err)
	if err != nil {
		return true
	}
	return false
}

func wrappedErrorPredicate(path string) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if wrapBeforeChecking(err) {
		return
	}
	defer file.Close()
}

func checkErrorWithDeferredResult(err error) (failed bool) {
	defer func() { failed = true }()
	if err != nil {
		return true
	}
	return false
}

func deferredErrorPredicate(path string) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if checkErrorWithDeferredResult(err) {
		return
	}
	defer file.Close()
}

func changedErrorPredicate(path string) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err == nil {
		err = errors.New("later failure")
	}
	if observeOpenError(err) {
		return
	}
	defer file.Close()
}

func immutableCapturedErrorPredicate(path string) {
	check := observeOpenError
	func() {
		file, err := os.Open(path)
		if check(err) {
			return
		}
		defer file.Close()
	}()
}

func mutableCapturedErrorPredicate(path string) {
	check := observeOpenError
	run := func() {
		file, err := os.Open(path) // want "owned resource from os.Open is not released"
		if check(err) {
			return
		}
		defer file.Close()
	}
	check = func(error) bool { return true }
	run()
}

func escapedCapturedErrorPredicate(path string, change func(*func(error) bool)) {
	check := observeOpenError
	run := func() {
		file, err := os.Open(path) // want "owned resource from os.Open is not released"
		if check(err) {
			return
		}
		defer file.Close()
	}
	change(&check)
	run()
}

func alternateCapturedErrorPredicate(path string, alternate bool) {
	check := observeOpenError
	if alternate {
		check = func(error) bool { return true }
	}
	func() {
		file, err := os.Open(path) // want "owned resource from os.Open is not released"
		if check(err) {
			return
		}
		defer file.Close()
	}()
}

func nestedImmutableErrorPredicate(path string) {
	check := observeOpenError
	func() {
		func() {
			file, err := os.Open(path)
			if check(err) {
				return
			}
			defer file.Close()
		}()
	}()
}

func nestedMutatedErrorPredicate(path string) {
	check := observeOpenError
	func() {
		run := func() {
			file, err := os.Open(path) // want "owned resource from os.Open is not released"
			if check(err) {
				return
			}
			defer file.Close()
		}
		check = func(error) bool { return true }
		run()
	}()
}

func nestedOpaqueErrorPredicate(path string, change func(*func(error) bool)) {
	check := observeOpenError
	func() {
		run := func() {
			file, err := os.Open(path) // want "owned resource from os.Open is not released"
			if check(err) {
				return
			}
			defer file.Close()
		}
		change(&check)
		run()
	}()
}

func recursiveErrorPredicate(path string, stop bool) {
	var check func(error) bool
	check = func(err error) bool {
		if stop {
			return true
		}
		return check(err)
	}
	func() {
		file, err := os.Open(path) // want "owned resource from os.Open is not released"
		if check(err) {
			return
		}
		defer file.Close()
	}()
}

func failedOneLine(err error) bool { return err != nil }

func oneLineErrorPredicate(path string) {
	file, err := os.Open(path)
	if failedOneLine(err) {
		return
	}
	defer file.Close()
}

// An exported predicate from another package carries the same implication
// through its result summary.
func importedErrorPredicate(path string) {
	file, err := os.Open(path)
	if resourcedep.Failed(err) {
		return
	}
	defer file.Close()
}

func importedUnrelatedErrorPredicate(path string, other error) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if resourcedep.FailedOther(err, other) {
		return
	}
	defer file.Close()
}
