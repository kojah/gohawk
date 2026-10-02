package resultfacts_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/resultfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
)

// Invalid wire or caller values must not become evidence for selecting a
// cleanup branch. Check the complete underlying domain, not only Unknown.
func TestGuaranteeOutcome(t *testing.T) {
	t.Parallel()
	known := map[resultfacts.Guarantee]ssaflow.Outcome{
		resultfacts.AlwaysNil: ssaflow.OutcomeNil, resultfacts.AlwaysNonNil: ssaflow.OutcomeNonNil,
		resultfacts.AlwaysTrue: ssaflow.OutcomeTrue, resultfacts.AlwaysFalse: ssaflow.OutcomeFalse,
	}
	for value := range 256 {
		guarantee := resultfacts.Guarantee(value)
		want, wantKnown := known[guarantee]
		if !wantKnown {
			want = ssaflow.OutcomeAny
		}
		got, gotKnown := guarantee.Outcome()
		if got != want || gotKnown != wantKnown {
			t.Errorf("guarantee %d: got (%v, %v), want (%v, %v)", value, got, gotKnown, want, wantKnown)
		}
	}
}
