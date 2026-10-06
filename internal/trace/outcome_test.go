package trace

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
)

func TestDiagnosticOutcomePolarity(t *testing.T) {
	expected := map[proofs.EvidenceState]Outcome{
		proofs.EvidenceProven:    OutcomeRejected,
		proofs.EvidenceDisproven: OutcomeAccepted,
	}
	for value := range 256 {
		state := proofs.EvidenceState(value)
		want, known := expected[state]
		if !known {
			want = OutcomeUnknown
		}
		if got := DiagnosticOutcome(state); got != want {
			t.Errorf("evidence %d: got %s, want %s", value, got, want)
		}
	}
}
