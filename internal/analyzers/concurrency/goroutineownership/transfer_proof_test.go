package goroutineownership

import "testing"

// These intentionally remain accepted by the precision-first reporter. The
// proof must still distinguish a possible handoff from exact ownership.
func TestTransferProofStrength(t *testing.T) {
	want := map[string]GoroutineOutcome{
		"mixedReturn":               GoroutineUnknown,
		"overwrittenAggregate":      GoroutineUnknown,
		"discardedWrapper":          GoroutineUnknown,
		"exactStore":                GoroutineLifecycleHonored,
		"mixedStore":                GoroutineUnknown,
		"overwrittenAggregateStore": GoroutineUnknown,
		"exactBesideOpaque":         GoroutineLifecycleHonored,
		"configuredMockResult":      GoroutineUnknown,
	}
	assertSpawnProofs(t, want, "transferlabels")
}
