package resourcelifetime

import (
	"os"
	"testing"
)

func reassignedCleanup(t *testing.T, name string) {
	f, err := os.Open(name)
	if err != nil {
		return
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			f.Close()
		}
	})
	closed = true
	f.Close()
	f, err = os.Open(name)
	if err != nil {
		return
	}
	closed = false
}

func cleanupCapturesDifferentVariable(t *testing.T, name string) {
	f, err := os.Open(name)
	if err != nil {
		return
	}
	t.Cleanup(func() { f.Close() })
	g, err := os.Open(name) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return
	}
	_ = g.Name()
}

func conditionalPriorCleanup(t *testing.T, name string, register bool) {
	var f *os.File
	if register {
		t.Cleanup(func() { f.Close() })
	}
	var err error
	f, err = os.Open(name) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return
	}
}

func boundCleanupKeepsOriginalValue(t *testing.T, name string) {
	f, err := os.Open(name)
	if err != nil {
		return
	}
	closeOriginal := f.Close
	t.Cleanup(func() { closeOriginal() })
	f, err = os.Open(name) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return
	}
	_ = f.Name()
}
