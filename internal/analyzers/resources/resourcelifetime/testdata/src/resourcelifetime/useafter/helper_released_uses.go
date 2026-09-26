package useafter

import (
	"os"
	"resourcedep"
)

// Released uses through helpers and forwarded flags. A helper proven to close
// the exact parameter on every return is the release, and a helper whose
// summary requires an operation on it is the use. A function that forwards
// its own flag to a helper with a latent released use has the same latent
// use, so a caller that fixes the flag is reported; one that passes the
// triggering literal itself is reported at that inner call, not again at
// its own callers.

func closeViaHelperThenRead(file *os.File) {
	closeFile(file)
	_, _ = file.Read(make([]byte, 1)) // want "parameter file is used after Close"
}

func closeThenReadViaHelper(file *os.File) {
	_ = file.Close()
	_ = Drain(file) // want "parameter file is used after Close"
}

func finishThenRead(file *os.File, keep bool) {
	finishFile(file, keep)
	_, _ = file.Read(make([]byte, 1))
}

func helperReleaseSelectedByFlag(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	finishThenRead(file, false) // want "finishThenRead calls Read after Close on this argument"
}

func helperReleaseNotSelected(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	finishThenRead(file, true)
	_ = file.Close()
}

func forwardRead(file *os.File, closeFirst bool) {
	readAfterMaybeClose(file, closeFirst)
}

func forwardedFlagTriggers(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	forwardRead(file, true) // want "forwardRead calls Read after Close on this argument"
}

func importedForwardedFlagTriggers(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	resourcedep.ForwardReadAfterMaybeClose(file, true) // want "ForwardReadAfterMaybeClose calls Read after Close on this argument"
}

func forwardedFlagDoesNotTrigger(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	forwardRead(file, false)
	_ = file.Close()
}

func forwardedVariableFlag(path string, closeFirst bool) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	forwardRead(file, closeFirst)
}

func alwaysReadAfterClose(file *os.File) {
	readAfterMaybeClose(file, true) // want "readAfterMaybeClose calls Read after Close on this argument"
}

func callsLiteralForwarder(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	alwaysReadAfterClose(file)
}
