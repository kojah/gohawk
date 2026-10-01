package trace

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
)

func TestDiagnosticOutcomePolarity(t *testing.T) {
	expected := map[ssaflow.EvidenceState]Outcome{
		ssaflow.EvidenceProven:    OutcomeRejected,
		ssaflow.EvidenceDisproven: OutcomeAccepted,
	}
	for value := range 256 {
		state := ssaflow.EvidenceState(value)
		want, known := expected[state]
		if !known {
			want = OutcomeUnknown
		}
		if got := DiagnosticOutcome(state); got != want {
			t.Errorf("evidence %d: got %s, want %s", value, got, want)
		}
	}
}
