package resourcelifetime

import (
	"io"
	"net/http"
	"os"
)

// Storage proofs must credit the saved resource, not whichever pointer its
// former slot contains now. These cases exercise aggregate copies emitted by
// SSA for literals as well as writes through equivalent field addresses.
type snapshotOwner struct{ file *os.File }

// Direct controls distinguish failures in acquisition tracking from failures
// introduced by storing the same resource in a field or array below.
func storageDirectLeak(path string) error {
	f, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	return f.Sync()
}

func storageDirectClosed(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func closeStoredField(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	x := snapshotOwner{file: f}
	defer x.file.Close()
	return x.file.Sync()
}

func closeStoredArray(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	x := [1]*os.File{f}
	defer x[0].Close()
	return x[0].Sync()
}

func closeSnapshotBeforeOverwrite(path string, replacement *os.File) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	x := snapshotOwner{file: f}
	saved := x.file
	x.file = replacement
	return saved.Close()
}

func closeCopiedOwner(path string, replacement *os.File) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	x := snapshotOwner{file: f}
	y := x
	x.file = replacement
	return y.file.Close()
}

func leakReplacedField(path string, replacement *os.File) error {
	f, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	x := snapshotOwner{file: f}
	alias := &x
	alias.file = replacement
	return x.file.Close()
}

func leakStoredField(path string) error {
	f, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	x := snapshotOwner{file: f}
	return x.file.Sync()
}

func leakStoredArray(path string) error {
	f, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	x := [1]*os.File{f}
	return x[0].Sync()
}

func readStoredResponse(response *http.Response) error {
	_, err := io.Copy(io.Discard, response.Body)
	return err
}

// Passing the owner to a reader prevents a local projection-stability proof.
// That uncertainty must not turn the subsequent explicit close into a leak.
func closeResponseAfterReader(url string) error {
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	err = readStoredResponse(response)
	_ = response.Body.Close()
	return err
}

func closeFieldAfterUnconditionalOverwrite(path string, other *os.File, pick bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	var x snapshotOwner
	if pick {
		x.file = f
	} else {
		x.file = other
	}
	x.file = f
	return x.file.Close()
}

func closeAgreeingFieldBranches(path string, pick bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	var x snapshotOwner
	if pick {
		x.file = f
	} else {
		x.file = f
	}
	return x.file.Close()
}

func closeSliceOffset(path string, other *os.File) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	x := [2]*os.File{other, f}
	return x[1:][0].Close()
}

func leakThroughWrongSliceOffset(path string, other *os.File) error {
	f, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	x := [2]*os.File{f, other}
	return x[1:][0].Close()
}
