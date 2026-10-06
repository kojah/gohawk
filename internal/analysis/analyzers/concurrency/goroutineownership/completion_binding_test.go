package goroutineownership

import "testing"

func TestCompletionBindingStrength(t *testing.T) {
	assertSpawnProofs(t, map[string]GoroutineOutcome{
		"mixedSignal":          GoroutineUnknown,
		"mixedGroup":           GoroutineUnknown,
		"replacedCapture":      GoroutineUnknown,
		"captureSnapshot":      GoroutineLifecycleHonored,
		"exactSignalMissing":   GoroutineLifecycleViolated,
		"exactSignalJoined":    GoroutineLifecycleHonored,
		"exactCaptureMissing":  GoroutineLifecycleViolated,
		"nestedCaptureMissing": GoroutineLifecycleViolated,
		"exactGroupMissing":    GoroutineLifecycleViolated,
	}, "completionbindings")
}
