package resourcelifetime

// An imported helper that closes its io.Closer parameter, directly or by
// passing the bound Close method to another helper, releases the caller's
// file: the call on the interface names the same method on the same object.
// A helper that accepts the closer and never closes it releases nothing.
// Real-world form: umoci's funchelpers.VerifyClose,
// https://github.com/opencontainers/umoci/blob/f5d1219acaf67127ebacf6306776d3ff465735ea/internal/funchelpers/verify_error.go#L55-L66

import (
	"os"

	"resourcedep"
)

func closedThroughInterface(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer resourcedep.CloseQuietly(file)
	_, err = file.WriteString("x")
	return err
}

func closedThroughMethodValue(path string) (err error) {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer resourcedep.RecordClose(&err, file)
	_, err = file.WriteString("x")
	return err
}

func ignoredThroughInterface(path string) error {
	file, err := os.Create(path) // want "owned resource from os.Create is not released on every return path"
	if err != nil {
		return err
	}
	defer resourcedep.Ignore(file)
	_, err = file.WriteString("x")
	return err
}
