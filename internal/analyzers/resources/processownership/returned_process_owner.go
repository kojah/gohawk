package processownership

import (
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Returning an aggregate that carries a projected process handle leaves its
// reaping ownership unknown. The completed body census and containment search
// share allowance; a truncated inventory cannot establish absence of an owner.
func proveReturnedProcessOwner(returned *ssa.Return, command ssa.Value, budget *ssaflow.SearchBudget) ssaflow.Proof {
	for instruction := range ssaflow.InstructionsWithin(returned.Parent(), budget) {
		handle, ok := instruction.(*ssa.UnOp)
		if !ok || !osProcessDerivedFromCommand(handle, command) {
			continue
		}
		owner := lifecycle.ProveReturnedOwnershipWithin(returned, handle, nil, budget)
		if owner.State != ssaflow.EvidenceDisproven {
			return owner
		}
	}
	return commandUseProof(false, budget)
}
