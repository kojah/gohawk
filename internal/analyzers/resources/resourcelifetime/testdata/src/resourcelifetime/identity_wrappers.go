package resourcelifetime

// A cleanup called on the result of a helper that returns its argument
// unchanged is an exact cleanup of the argument: the result summary proves
// the identity for local and imported helpers alike, so the release is
// settled rather than left as an ambiguous cleanup value. A helper that
// returns some other value, or erases the type, has no such identity; its
// cleanup stays the ambiguous boundary it always was, an unknown that hides
// the diagnostic without proving release, which is why no diagnostic form
// exists for this file.

import (
	"io"
	"os"

	"resourcedep"
)

func sameFile(file *os.File) *os.File { return file }

func closedThroughLocalPassThrough(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer sameFile(file).Close()
	return nil
}

func closedThroughImportedPassThrough(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer resourcedep.Unchanged(file).Close()
	return nil
}

func closedThroughReplacingHelperStaysAmbiguous(path string, other *os.File) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer resourcedep.Replaced(file, other).Close()
	return nil
}

func closedThroughErasingHelperStaysAmbiguous(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer resourcedep.Erased(file).(io.Closer).Close()
	return nil
}
