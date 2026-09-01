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

func namedTestCallback(t *testing.T) {
	_ = t
}

func namedBenchmarkCallback(b *testing.B) {
	_ = b
}

func mixedCallbackAndHelper(t *testing.T) { // want "test helper accepting t"
	_ = t
}

func retainBenchmarkCallback(callback func(*testing.B)) {
	_ = callback
}

func TestEntryPoint(t *testing.T) {
	_ = t
}

func TestCallbacksAreNotHelpers(t *testing.T) {
	t.Run("named callback", namedTestCallback)
	t.Run("mixed callback", mixedCallbackAndHelper)
	mixedCallbackAndHelper(t)
	t.Run("callback", func(t *testing.T) {
		_ = t
	})
	builders := []func(*testing.T){
		func(t *testing.T) { _ = t },
	}
	builders[0](t)
}

func BenchmarkNamedCallback(b *testing.B) {
	retainBenchmarkCallback(namedBenchmarkCallback)
	_ = b
}
