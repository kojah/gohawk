package processownership

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// This file turns the post-Start flow result into the final reporting decision.
// An unowned return is insufficient when the handle is unused: detached launch
// intent remains unknown. Tracing consumes the same decision as reporting.
type processReturnDecision struct {
	state  ssaflow.EvidenceState
	reason processReason
}

func decideProcessReturn(start *ssa.Call, command ssa.Value, witness *ssa.Return, unknown bool) processReturnDecision {
	if witness == nil {
		if unknown {
			return processReturnDecision{ssaflow.EvidenceUnknown, reasonAmbiguousWaitOwnership}
		}
		return processReturnDecision{ssaflow.EvidenceDisproven, reasonWaitOwnershipProven}
	}
	// Fire-and-forget alone cannot distinguish an intentional browser or
	// daemon launch from a defect. Retiring the detached audit must not
	// broaden missing-wait to report those same uncertain launches.
	if commandUnusedAfterStart(start, command) {
		return processReturnDecision{ssaflow.EvidenceUnknown, reasonUnusedCommandOwnershipUnknown}
	}
	return processReturnDecision{ssaflow.EvidenceProven, reasonUnownedReturn}
}

func emitProcessDecision(pass *analysis.Pass, function *ssa.Function, start *ssa.Call, command ssa.Value, decision processReturnDecision) {
	checkID := string(check.ProcessWait)
	if !analysisTrace.Enabled("processownership", checkID) {
		return
	}
	outcome := analysisTrace.OutcomeUnknown
	switch decision.state {
	case ssaflow.EvidenceProven:
		outcome = analysisTrace.OutcomeRejected
	case ssaflow.EvidenceDisproven:
		outcome = analysisTrace.OutcomeAccepted
	case ssaflow.EvidenceUnknown:
	}
	details := map[string]string{}
	if command != nil && command.Type() != nil {
		details["command_type"] = command.Type().String()
	}
	analysisTrace.For(pass, "processownership", checkID, start.Pos()).Decision(analysisTrace.Step{
		Reason:   decision.reason.String(),
		Outcome:  outcome,
		Pos:      start.Pos(),
		Function: function.String(),
		Details:  details,
	})
}
