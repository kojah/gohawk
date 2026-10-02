package lifecyclefacts

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Declaration masks become call-site claims through exact storage identity or
// the existing guarded containment relation. Bounded queries preserve storage
// cutoff as unknown; ambiguous aliasing cannot become exact target coverage.

// factOwnsExactArgument is factOwnsArgument without containment: only the
// target itself passed as the masked argument counts, so a literal that
// captured the target is not mistaken for it.
func factOwnsExactArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask) bool {
	return factArgumentMatches(instruction, target, mask, func(value, target ssa.Value) bool {
		return heapmodel.NewStorage(nil).Same(value, target).Proven()
	})
}

func factArgumentMatches(instruction ssa.Instruction, target ssa.Value, mask ParameterMask, matches func(ssa.Value, ssa.Value) bool) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for index, argument := range common.Args {
		if mask.contains(index) && matches(argument, target) {
			return true
		}
	}
	return false
}

// factOwnsArgument reports whether mask covers the argument which contains
// target at this callsite.
func factOwnsArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask, observer ssaflow.Observer) bool {
	return proveFactOwnsArgumentWithin(instruction, target, mask, observer, nil).Proven()
}

func proveFactOwnsArgumentWithin(
	instruction ssa.Instruction, target ssa.Value,
	mask ParameterMask, observer ssaflow.Observer, budget *ssaflow.SearchBudget,
) ssaflow.Proof {
	missing := ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return missing
	}
	for index, argument := range common.Args {
		if !budget.Spend() {
			return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
		}
		if !mask.contains(index) {
			continue
		}
		storageBudget := budget.Within(ssaflow.QueryBudget).Observed(observer)
		same := heapmodel.NewStorage(storageBudget).Same(argument, target)
		// Default callers retain their previous independent storage cap and
		// containment fallback. A bounded caller must preserve cutoff availability.
		if budget != nil && (storageBudget.Exhausted() || storageBudget.PoolExhausted()) {
			return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
		}
		if same.Proven() {
			return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceStructuralWalk}
		}
		// Containment cannot turn an ambiguous phi or storage-history match into
		// a guarantee about this exact target. Keep the existing alias exclusion.
		if !budget.Spend() {
			return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
		}
		if !heapmodel.MayAlias(argument, target) {
			contains := lifecycle.ProveMayContainValueWithin(argument, target, budget)
			if contains.State == ssaflow.EvidenceUnknown || contains.Proven() {
				return contains
			}
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
	}
	return missing
}

func factOwnsProjectedArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask, observer ssaflow.Observer) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for index, argument := range common.Args {
		storage := heapmodel.NewStorage(ssaflow.NewSearchBudget(ssaflow.QueryBudget).Observed(observer))
		if mask.contains(index) && storage.Projection(argument, target, instruction).Proven() {
			return true
		}
	}
	return false
}
