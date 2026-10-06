package resultfacts_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/resultfacts"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
)

// Invalid wire or caller values must not become evidence for selecting a
// cleanup branch. Check the complete underlying domain, not only Unknown.
func TestGuaranteeOutcome(t *testing.T) {
	t.Parallel()
	known := map[resultfacts.Guarantee]ssacall.Outcome{
		resultfacts.AlwaysNil: ssacall.OutcomeNil, resultfacts.AlwaysNonNil: ssacall.OutcomeNonNil,
		resultfacts.AlwaysTrue: ssacall.OutcomeTrue, resultfacts.AlwaysFalse: ssacall.OutcomeFalse,
	}
	for value := range 256 {
		guarantee := resultfacts.Guarantee(value)
		want, wantKnown := known[guarantee]
		if !wantKnown {
			want = ssacall.OutcomeAny
		}
		got, gotKnown := guarantee.Outcome()
		if got != want || gotKnown != wantKnown {
			t.Errorf("guarantee %d: got (%v, %v), want (%v, %v)", value, got, gotKnown, want, wantKnown)
		}
	}
}
