package testpolicyfix

import "testing"

func requireUser(t *testing.T) { // want "test helper accepting t"
	t.Log("direct testing use")
}

func emptyHelper(t *testing.T) {}

func conditionalHelper(t *testing.T, enabled bool) { // want "on every return path"
	if enabled {
		t.Helper()
	}
}
