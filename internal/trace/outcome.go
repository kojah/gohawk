package trace

import "github.com/kojah/gohawk/internal/ssaflow"

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
