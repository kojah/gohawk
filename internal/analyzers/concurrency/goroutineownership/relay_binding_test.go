package goroutineownership

import "testing"

func TestRelayBindingStrength(t *testing.T) {
	assertSpawnProofs(t, map[string]GoroutineOutcome{
		"reassignedGroup":     GoroutineLifecycleHonored,
		"exactRelay":          GoroutineLifecycleHonored,
		"unrelatedWait":       GoroutineLifecycleViolated,
		"relayWithCallerWork": GoroutineUnknown,
		"relayWithWork":       GoroutineLifecycleViolated,
	}, "relaybindings")
}
