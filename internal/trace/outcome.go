package trace

import (
	"github.com/kojah/gohawk/internal/enumtext"
	"github.com/kojah/gohawk/internal/ssaflow"
)

// DiagnosticOutcome presents evidence whose proven proposition is that a
// diagnostic may be reported: proven is rejected, disproven is accepted, and
// unknown or invalid evidence stays unknown. It does not decide whether to
// report. Cleanup or transfer proofs with the opposite polarity must not use it.
func DiagnosticOutcome(state ssaflow.EvidenceState) Outcome {
	switch state {
	case ssaflow.EvidenceProven:
		return OutcomeRejected
	case ssaflow.EvidenceDisproven:
		return OutcomeAccepted
	case ssaflow.EvidenceUnknown:
		return OutcomeUnknown
	default:
		return OutcomeUnknown
	}
}

// Outcome is the result of an evidence decision. Zero remains unset rather
// than claiming an observation or an unknown decision occurred.
type Outcome uint8

const (
	_ Outcome = iota
	OutcomeObserved
	OutcomeAccepted
	OutcomeRejected
	OutcomeUnknown
)

var outcomeLabels = [...]string{0: "", OutcomeObserved: "observed", OutcomeAccepted: "accepted", OutcomeRejected: "rejected", OutcomeUnknown: "unknown"}

// String returns the stable presentation label.
func (value Outcome) String() string { return enumtext.Name(value, outcomeLabels[:]) }

// MarshalText preserves string labels in text and JSON output.
func (value Outcome) MarshalText() ([]byte, error) { return enumtext.Encode(value, outcomeLabels[:]) }

// UnmarshalText accepts only domain labels and leaves value unchanged on error.
func (value *Outcome) UnmarshalText(text []byte) error {
	return enumtext.Decode(value, text, outcomeLabels[:])
}
