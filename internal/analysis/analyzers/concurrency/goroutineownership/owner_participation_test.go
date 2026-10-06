package goroutineownership

import "testing"

func TestOwnerParticipationStrength(t *testing.T) {
	want := map[string]GoroutineOutcome{
		"directClose":    GoroutineUnknown,
		"deferredClose":  GoroutineUnknown,
		"helperClose":    GoroutineUnknown,
		"directStop":     GoroutineUnknown,
		"directShutdown": GoroutineUnknown,
		"directWait":     GoroutineUnknown,
		"directKill":     GoroutineUnknown,
		"closeAndJoin":   GoroutineLifecycleHonored,
		"closeOther":     GoroutineLifecycleViolated,
	}
	assertSpawnProofs(t, want, "ownerparticipation")
}
