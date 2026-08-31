package testpolicy

import "testing"

func missingHelper(t *testing.T) { // want "test helper accepting t"
	_ = t
}

func hasHelper(t *testing.T) {
	t.Helper()
}

func hasHelperThroughAlias(t *testing.T) {
	alias := t
	alias.Helper()
}

func conditionalHelper(t *testing.T, enabled bool) { // want "on every return path"
	if enabled {
		t.Helper()
	}
}

func TestEntryPoint(t *testing.T) {
	_ = t
}
