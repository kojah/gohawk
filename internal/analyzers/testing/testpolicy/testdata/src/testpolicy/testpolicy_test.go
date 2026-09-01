package testpolicy

import "testing"

func missingHelper(t *testing.T) { // want "test helper accepting t"
	t.Log("direct testing use")
}

func unusedTestingHandle(t *testing.T) {}

func unusedBlankTestingHandle(_ *testing.T) {}

func markedForwardedHelper(t *testing.T) {
	t.Helper()
}

func forwardsTestingHandle(t *testing.T) { // want "test helper accepting t"
	markedForwardedHelper(t)
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

type callbackSuite struct{}

func (*callbackSuite) methodCallback(t *testing.T) {
	t.Error("callback failure")
}

func (*callbackSuite) mixedMethodCallback(t *testing.T) { // want "test helper accepting t"
	t.Error("mixed callback failure")
}

func (*callbackSuite) escapedMethodCallback(t *testing.T) { // want "test helper accepting t"
	t.Error("escaped callback failure")
}

func (*callbackSuite) nonTestingMethodCallback(t *testing.T) { // want "test helper accepting t"
	t.Error("non-testing callback failure")
}

func retainAnyCallback(callback any) {
	_ = callback
}

func mixedCallbackAndHelper(t *testing.T) { // want "test helper accepting t"
	t.Error("mixed callback failure")
}

func returnedTestingCallback(t *testing.T) func() {
	return func() { t.Error("callback failure") }
}

func mixedReturnedCallbackAndHelper(t *testing.T) func() { // want "test helper accepting t"
	t.Log("factory called")
	return func() { t.Error("callback failure") }
}

func invokedAndReturnedTestingCallback(t *testing.T) func() { // want "test helper accepting t"
	callback := func() { t.Error("callback failure") }
	callback()
	return callback
}

func retainBenchmarkCallback(callback func(*testing.B)) {
	_ = callback
}

func TestEntryPoint(t *testing.T) {
	_ = t
}

func TestCallbacksAreNotHelpers(t *testing.T) {
	suite := &callbackSuite{}
	_ = returnedTestingCallback(t)
	_ = mixedReturnedCallbackAndHelper(t)
	_ = invokedAndReturnedTestingCallback(t)
	t.Run("named callback", namedTestCallback)
	t.Run("method callback", suite.methodCallback)
	t.Run("mixed method callback", suite.mixedMethodCallback)
	suite.mixedMethodCallback(t)
	escaped := suite.escapedMethodCallback
	_ = escaped
	retainAnyCallback(suite.nonTestingMethodCallback)
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
