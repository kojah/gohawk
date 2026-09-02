package resourcelifetime

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"resourcedep"
	"testing"
)

func importedHelperClosesFile() error {
	file, err := os.Open("fixture")
	if err != nil {
		return err
	}
	return resourcedep.Close(file)
}

func conditionalImportedHelperLeaksFile(enabled bool) error {
	file, err := os.Open("fixture") // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	return resourcedep.MaybeClose(file, enabled)
}

func leakedFile() error {
	file, err := os.CreateTemp("", "leak") // want "owned resource from os.CreateTemp is not released on every return path"
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func closedFile() error {
	file, err := os.CreateTemp("", "closed")
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

func closedFileThroughFailureHelper(fail bool) error {
	file, err := os.CreateTemp("", "closed-helper")
	if err != nil {
		return err
	}
	cleanup := func(err error) error {
		_ = file.Close()
		return err
	}
	if fail {
		return cleanup(errors.New("failed"))
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

func leakedWriterAsInterface() error {
	file, err := os.CreateTemp("", "writer") // want "owned resource from os.CreateTemp is not released on every return path"
	if err != nil {
		return err
	}
	var destination io.Writer = file
	_, err = destination.Write(nil)
	return err
}

func closedFileAfterIgnoredMissingPath(paths []string) error {
	for _, path := range paths {
		file, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

func closedFileAfterIgnoredFSMissingPath(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return file.Close()
}

func closedFileAfterLegacyMissingPath(path string) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return file.Close()
}

func closedFileAfterTypedErrorCheck(path string) error {
	file, err := os.Open(path)
	if pathError, ok := err.(*os.PathError); ok && errors.Is(pathError.Err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return file.Close()
}

func leakedFileAfterTypedErrorCheck(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if _, ok := err.(*os.PathError); ok {
		return err
	}
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func unrelatedTypedErrorDoesNotSettleFile(path string, other error) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if _, ok := other.(*os.PathError); ok {
		return other
	}
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func closedFileAfterExistingPath(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return file.Close()
}

func transferredFileAfterLegacyExistingPath(path string) (*os.File, error) {
	for attempts := 0; ; attempts++ {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			if attempts < 2 {
				continue
			}
			return nil, err
		}
		return file, err
	}
}

func leakedFileAfterLegacyExistingPath(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // want "owned resource from os.OpenFile is not released on every return path"
	if os.IsExist(err) {
		return err
	}
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func leakedFileAfterExistingPath(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // want "owned resource from os.OpenFile is not released on every return path"
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func leakedFileOnNegatedExistingCheck(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // want "owned resource from os.OpenFile is not released on every return path"
	if !errors.Is(err, fs.ErrExist) {
		_ = file
		return nil
	}
	return err
}

func settleFileParameter(file *os.File) {
	_ = file.Close()
}

func fileClosedByHelper() error {
	file, err := os.Open("state")
	if err != nil {
		return err
	}
	settleFileParameter(file)
	return nil
}

func fileTransferredToReceiver(files chan<- *os.File) error {
	file, err := os.Open("state")
	if err != nil {
		return err
	}
	files <- file
	return nil
}

func fileClosedByTestCleanup(t *testing.T) error {
	file, err := os.CreateTemp(t.TempDir(), "cleanup")
	if err != nil {
		return err
	}
	t.Cleanup(func() { _ = file.Close() })
	return nil
}

func fileConditionallyClosedByTestCleanup(t *testing.T, closeFile bool) error {
	file, err := os.CreateTemp(t.TempDir(), "conditional-cleanup") // want "owned resource from os.CreateTemp is not released on every return path"
	if err != nil {
		return err
	}
	t.Cleanup(func() {
		if closeFile {
			_ = file.Close()
		}
	})
	return nil
}

func transferredFile() (*os.File, error) {
	return os.CreateTemp("", "transfer")
}

var packageFile *os.File

func transferredFileToPackageOwner() error {
	file, err := os.Open("state")
	if err != nil {
		return err
	}
	packageFile = file
	return nil
}

func closedFileThroughDeferredParameter() error {
	file, err := os.CreateTemp("", "closed")
	if err != nil {
		return err
	}
	defer func(open *os.File) {
		_ = open.Close()
	}(file)
	return nil
}

func acquiredForEnclosingScope() func() error {
	var file *os.File
	load := func() error {
		var err error
		file, err = os.Open("fixture")
		return err
	}
	_ = load()
	return file.Close
}
