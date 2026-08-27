package testpolicyfix

import "testing"

func requireUser(t *testing.T) { // want "test helper accepting t"
	_ = t
}

func emptyHelper(t *testing.T) {} // want "test helper accepting t"

func conditionalHelper(t *testing.T, enabled bool) { // want "on every return path"
	if enabled {
		t.Helper()
	}
}
