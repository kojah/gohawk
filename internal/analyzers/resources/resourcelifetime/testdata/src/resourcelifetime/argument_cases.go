package resourcelifetime

import (
	"bytes"
	"io"
	"net/http"
	"os"

	"resourcedep"
	"resourceforward"
)

// Argument cases. A helper that closes only behind a Boolean parameter settles
// the file at a call whose argument is a constant selecting the closing
// branch. A variable or merged flag still leaves the file open on some path,
// and a constant selecting the other branch proves the leak.

func finishFile(file *os.File, keep bool) {
	if !keep {
		file.Close()
	}
}

func finishFileUnlessKept(file *os.File, keep bool) {
	if keep {
		return
	}
	file.Close()
}

func constantFlagClosesLocally(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	finishFile(file, false)
	return nil
}

func constantFlagKeepsLocally(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	finishFile(file, true)
	return nil
}

func earlyReturnHelperClosesLocally(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	finishFileUnlessKept(file, false)
	return nil
}

func variableFlagStillLeaks(path string, keep bool) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	finishFile(file, keep)
	return nil
}

func mergedFlagStillLeaks(path string, quick bool) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	keep := false
	if quick {
		keep = true
	}
	finishFile(file, keep)
	return nil
}

func finishFileNow(file *os.File) { finishFile(file, false) }

func constantFixedByLocalWrapper(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	finishFileNow(file)
	return nil
}

func finishFileAs(file *os.File, keep bool) { finishFile(file, keep) }

func constantBoundThroughParameterChain(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	finishFileAs(file, false)
	return nil
}

func deferredConstantCloses(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer finishFile(file, false)
	return nil
}

func deferredConstantKeeps(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	defer finishFile(file, true)
	return nil
}

func constantFlagClosesImported(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return resourcedep.MaybeClose(file, true)
}

func constantFlagKeepsImported(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	return resourcedep.MaybeClose(file, false)
}

func earlyReturnHelperClosesImported(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	resourcedep.CloseUnlessKept(file, false)
	return nil
}

func bothFlagsCloseImported(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	resourcedep.CloseWhenBoth(file, true, true)
	return nil
}

func oneFlagKeepsImported(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	resourcedep.CloseWhenBoth(file, true, false)
	return nil
}

func constantFixedByImportedWrapper(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return resourceforward.CloseFile(file)
}

func constantKeptByImportedWrapper(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	return resourceforward.KeepFile(file)
}

// The helper's deferred closure captures the flag rather than testing the
// parameter directly.
func capturedConstantClosesResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	resourcedep.MaybeCloseResponse(response, true)
	return nil
}

func capturedConstantKeepsResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	resourcedep.MaybeCloseResponse(response, false)
	return nil
}

// A flag named after closing that guards something else is not a cleanup
// case, whatever its value.
func misleadingFlagName(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	resourcedep.TouchUnlessQuiet(file, true)
	return nil
}

// Nil arguments select cases too. A nil literal is nil; an allocation is
// not; and an interface holding a typed nil pointer is not a nil interface.

type fileOptions struct{ keep bool }

func closeWithoutOptions(file *os.File, options *fileOptions) {
	if options == nil {
		file.Close()
	}
}

func nilOptionsClose(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	closeWithoutOptions(file, nil)
	return nil
}

func allocatedOptionsKeep(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	closeWithoutOptions(file, &fileOptions{keep: true})
	return nil
}

func variableOptionsKeep(path string, options *fileOptions) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	closeWithoutOptions(file, options)
	return nil
}

func nilOptionsCloseImported(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	resourcedep.CloseWithoutOptions(file, nil)
	return nil
}

func allocatedOptionsKeepImported(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	resourcedep.CloseWithoutOptions(file, &resourcedep.Options{})
	return nil
}

func closeWithoutWriter(file *os.File, writer io.Writer) {
	if writer == nil {
		file.Close()
	}
}

func nilWriterCloses(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	closeWithoutWriter(file, nil)
	return nil
}

func typedNilWriterKeeps(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	var buffer *bytes.Buffer
	closeWithoutWriter(file, buffer)
	return nil
}
