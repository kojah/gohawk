package resourcelifetime

import (
	"os"
	"resourcedep"
)

// An imported helper's every-return method requirement can settle the exact
// resource even when Close is invoked through an interface one helper deeper.
func deferredRequiredClose(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer resourcedep.CloseThroughInterface(file)
	return nil
}

func calledRequiredClose(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	resourcedep.CloseThroughInterface(file)
	return nil
}

func observedButNotClosed(path string) error {
	file, err := os.Create(path) // want "owned resource from os.Create is not released on every return path"
	if err != nil {
		return err
	}
	resourcedep.ObserveCloser(file)
	return nil
}
