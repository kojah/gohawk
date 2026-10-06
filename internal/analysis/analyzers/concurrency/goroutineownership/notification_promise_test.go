package goroutineownership

import "testing"

func TestNotificationPromiseStrength(t *testing.T) {
	assertSpawnProofs(t, map[string]GoroutineOutcome{
		"optionalClose":        GoroutineUnknown,
		"optionalSend":         GoroutineUnknown,
		"optionalRegistration": GoroutineUnknown,
		"nestedProgress":       GoroutineUnknown,
		"outerProgress":        GoroutineUnknown,
		"detachedClose":        GoroutineUnknown,
		"alternateMissing":     GoroutineLifecycleViolated,
		"deferredMissing":      GoroutineLifecycleViolated,
		"synchronousMissing":   GoroutineLifecycleViolated,
		"deferredJoined":       GoroutineLifecycleHonored,
		"synchronousJoined":    GoroutineLifecycleHonored,
	}, "notificationpromises")
}
