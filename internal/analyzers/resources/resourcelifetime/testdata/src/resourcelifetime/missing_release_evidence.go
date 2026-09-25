package resourcelifetime

import (
	"errors"
	"os"
)

// These cases pin the evidence a missing-release diagnostic cites: the
// return the flow walk reached with the resource still owed, labeled with the
// variable that holds it, or the end of a function that has no return.

func evidenceAtReturn(path string) error {
	config, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return err
	}
	if config.Name() == "" {
		return errors.New("unnamed")
	}
	return config.Close()
}

func evidenceAtFunctionEnd(path string) {
	_, _ = os.Open(path) // want "owned resource from os.Open is not released"
}
