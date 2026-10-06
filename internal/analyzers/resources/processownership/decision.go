package processownership

import (
	"github.com/kojah/gohawk/internal/check"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// This file owns final process decision presentation and post-Start policy.
// Pre-Start suppressions and post-Start results share the same trace projection.
// An unowned return is insufficient when the handle is unused: detached launch
// intent remains unknown. Tracing consumes the same decision as reporting.
type processDecision struct {
	state  proofs.EvidenceState
	reason processReason
}

func decideProcessReturn(start *ssa.Call, command ssa.Value, witness *ssa.Return, unknown bool, budget *proofs.SearchBudget) processDecision {
	if witness == nil {
		if unknown {
			return processDecision{proofs.EvidenceUnknown, reasonAmbiguousWaitOwnership}
		}
		return processDecision{proofs.EvidenceDisproven, reasonWaitOwnershipProven}
	}
	// Fire-and-forget alone cannot distinguish an intentional browser or
	// daemon launch from a defect. Retiring the detached audit must not
	// broaden missing-wait to report those same uncertain launches.
	use := proveCommandUseAfterStart(start, command, budget)
	if use.State == proofs.EvidenceUnknown {
		return processDecision{proofs.EvidenceUnknown, reasonCommandUseCutoff}
	}
	if use.State == proofs.EvidenceDisproven {
		return processDecision{proofs.EvidenceUnknown, reasonUnusedCommandOwnershipUnknown}
	}
	// A one-time start in the executable's entry may be owned until program
	// exit. An uncovered entry return cannot distinguish that lifetime from
	// missing reaping. This declines the report; it does not credit Kill as
	// Wait or assert that parent exit terminates the child. Repeated starts,
	// referenced entries and reusable callees remain ordinary wait obligations.
	// https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/example/agent/main.go#L401-L423
	if ssaflow.RunsOnceInProgramEntry(start) {
		return processDecision{proofs.EvidenceUnknown, reasonProgramLifetimeOwnershipUnknown}
	}
	return processDecision{proofs.EvidenceProven, reasonUnownedReturn}
}

func emitProcessDecision(pass *analysis.Pass, function *ssa.Function, start *ssa.Call, command ssa.Value, decision processDecision) {
	checkID := string(check.ProcessWait)
	if !analysisTrace.Enabled("processownership", checkID) {
		return
	}
	outcome := analysisTrace.DiagnosticOutcome(decision.state)
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
