package lockorder

import (
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

// Both caller preconditions require complete private function uses, including
// initialization and generated bodies. An opaque use or an interrupted census
// cannot establish either cleanup coverage or exclusive parameter ownership.
type conditionalCallerSet = ssacall.PrivateFunctionUses

// callerSetBudget bounds the whole-package caller census rather than one
// candidate. Exhaustion invalidates every caller set for both consumers.
const callerSetBudget = 20_000

func collectLockCallers(initialization *ssa.Function, functions []*ssa.Function, budget *proofs.SearchBudget) map[*ssa.Function]conditionalCallerSet {
	return ssacall.CollectPrivateFunctionUsesWithin(append([]*ssa.Function{initialization}, functions...), budget)
}
