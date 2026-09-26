package useafter

import (
	"database/sql"
	"os"
	"resourcedep"
)

// Released uses inside a function. A function that closes its parameter and
// then operates on it is wrong whoever calls it, so the use is reported where
// it happens. One that does so only when a Boolean parameter selects it is
// wrong only at a call that passes that constant, so the call is reported and
// the function itself is not. The release must be a direct call on the exact
// parameter that dominates the use on the paths the condition allows, with
// nothing else touching the parameter in between.

func closeThenRead(file *os.File) {
	_ = file.Close()
	_, _ = file.Read(make([]byte, 1)) // want "parameter file is used after Close"
}

func readAfterMaybeClose(file *os.File, closeFirst bool) {
	if closeFirst {
		_ = file.Close()
	}
	_, _ = file.Read(make([]byte, 1))
}

func latentUseAtTriggeringCall(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	readAfterMaybeClose(file, true) // want "readAfterMaybeClose calls Read after Close on this argument"
}

func latentUseAtImportedTriggeringCall(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	resourcedep.ReadAfterMaybeClose(file, true) // want "ReadAfterMaybeClose calls Read after Close on this argument"
}

func latentUseNotTriggered(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	readAfterMaybeClose(file, false)
	_ = file.Close()
}

func latentUseWithVariableFlag(path string, closeFirst bool) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	readAfterMaybeClose(file, closeFirst)
}

// The release depends on the function's own data, not on a caller's
// constant, and does not dominate the read.
func releaseOnInternalBranch(file *os.File, size int) {
	if size > 0 {
		_ = file.Close()
	}
	_, _ = file.Read(make([]byte, 1))
}

func reopenedParameter(file *os.File, path string) {
	_ = file.Close()
	file, err := os.Open(path)
	if err != nil {
		return
	}
	_, _ = file.Read(make([]byte, 1))
	_ = file.Close()
}

func inspect(file *os.File) { _ = file.Name() }

// Anything else that touches the parameter between the release and the use
// could change what it holds; the claim needs nothing in between.
func helperBetweenReleaseAndUse(file *os.File) {
	_ = file.Close()
	inspect(file)
	_, _ = file.Read(make([]byte, 1))
}

// Err after Close is documented as harmless, and a second Close is not an
// operation that fails on a released value.
func errAfterParameterClose(rows *sql.Rows) error {
	_ = rows.Close()
	return rows.Err()
}

func closeTwice(file *os.File) {
	_ = file.Close()
	_ = file.Close()
}

func readBeforeDeferredClose(file *os.File) {
	defer file.Close()
	_, _ = file.Read(make([]byte, 1))
}

func closeOneReadOther(first, second *os.File) {
	_ = first.Close()
	_, _ = second.Read(make([]byte, 1))
}

// A flag named after closing that guards something else selects nothing.
func touchUnlessClosed(file *os.File, closed bool) {
	if closed {
		inspect(file)
	}
	_, _ = file.Read(make([]byte, 1))
}

func misleadingFlag(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	touchUnlessClosed(file, true)
	_ = file.Close()
}

// The manifest bug is reported once, inside closeThenRead, not again at
// every call.
func callsManifestHelper(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	closeThenRead(file)
}

type readOptions struct{ keep bool }

func readAfterCloseWithoutOptions(file *os.File, options *readOptions) {
	if options == nil {
		_ = file.Close()
	}
	_, _ = file.Read(make([]byte, 1))
}

func nilOptionsTriggerLatentUse(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	readAfterCloseWithoutOptions(file, nil) // want "readAfterCloseWithoutOptions calls Read after Close on this argument"
}

func allocatedOptionsDoNotTrigger(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	readAfterCloseWithoutOptions(file, &readOptions{})
	_ = file.Close()
}

func nilOptionsTriggerImportedLatentUse(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	resourcedep.ReadAfterCloseWithoutOptions(file, nil) // want "ReadAfterCloseWithoutOptions calls Read after Close on this argument"
}
