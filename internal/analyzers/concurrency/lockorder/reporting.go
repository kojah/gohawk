package lockorder

import (
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
)

// lockDiagnosticProof is the final policy result consumed by reporting and
// tracing. Proven permits a diagnostic, disproven excludes it, and unknown
// suppresses it without establishing release or protection of the written field.
// Each check owns its evidence rules; this file only presents their outcomes.
type lockDiagnosticProof struct {
	state  ssaflow.EvidenceState
	reason lockReason
}

func traceLockDiagnostic(pass *analysis.Pass, id check.ID, position token.Pos, proof lockDiagnosticProof) {
	// Instructions with no matching obligation or owner are not candidates.
	if proof.reason == lockReasonNone {
		return
	}
	outcome := analysisTrace.OutcomeUnknown
	switch proof.state {
	case ssaflow.EvidenceProven:
		outcome = analysisTrace.OutcomeRejected
	case ssaflow.EvidenceDisproven:
		outcome = analysisTrace.OutcomeAccepted
	case ssaflow.EvidenceUnknown:
	}
	analysisTrace.For(pass, "lockorder", string(id), position).Decision(analysisTrace.Step{
		Reason: proof.reason.String(), Outcome: outcome, Pos: position,
	})
}
