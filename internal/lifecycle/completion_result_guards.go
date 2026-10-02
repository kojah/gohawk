package lifecycle

import (
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Result-guarded deferred completion. A deferred literal runs after the
// return statement has set the function's named results, so a literal that
// completes the target only while a named result holds some outcome, as the
// close-on-error idiom closes only while err is non-nil, completes on some
// returns and not on others. Asking once, at the defer, can only say "may"
// or "never". These queries ask per return instead: which deferred literals
// turn on a named result, and whether one completes the target given the
// value a particular return stores. Whether an unknown value suppresses a
// diagnostic, and what else settles the obligation on that path, is the
// caller's policy.

// ResultGuard is a deferred literal whose completion of a target turns on
// named results of the function that defers it.
type ResultGuard struct {
	Defer *ssa.Defer
	// Cells are the named-result cells the literal captures.
	Cells []*ssa.Alloc
}

// ResultGuardsProof publishes only a complete census of modeled result guards.
// Proven means discovery completed, including when Guards is empty; it does
// not assert completion at any return of the enclosing function.
type ResultGuardsProof struct {
	ssaflow.Proof
	Guards []ResultGuard
}

// ProveResultGuards shares request.Budget across instruction, capture,
// named-result and opposing completion questions. Cutoff discards all guards;
// completed opaque completion answers retain the ordinary discovery policy.
func ProveResultGuards(function *ssa.Function, request CompletionRequest) ResultGuardsProof {
	unknown := ResultGuardsProof{Proof: ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}}
	if !request.Budget.Spend() {
		return unknown
	}
	var guards []ResultGuard
	for instruction := range ssaflow.InstructionsWithin(function, request.Budget) {
		deferred, ok := instruction.(*ssa.Defer)
		if !ok {
			continue
		}
		closure, ok := deferred.Call.Value.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		var cells []*ssa.Alloc
		for _, binding := range closure.Bindings {
			if !request.Budget.Spend() {
				return unknown
			}
			if cell, ok := binding.(*ssa.Alloc); ok {
				if _, named := ssaflow.NamedResultCellWithin(function, cell, request.Budget); named {
					cells = append(cells, cell)
				}
			}
		}
		guard := ResultGuard{Defer: deferred, Cells: cells}
		if len(cells) != 0 && guard.turnsOnResult(request) {
			guards = append(guards, guard)
		}
		// Neither a missing guard nor a partial positive list is authoritative
		// when either the census or an opposing completion query stopped early.
		if request.Budget.Exhausted() || request.Budget.PoolExhausted() {
			return unknown
		}
	}
	if request.Budget.Exhausted() || request.Budget.PoolExhausted() {
		return unknown
	}
	return ResultGuardsProof{
		Proof:  ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceStructuralWalk, Provenance: ssaflow.EvidenceFromLocalSSA},
		Guards: guards,
	}
}

func (guard ResultGuard) turnsOnResult(request CompletionRequest) bool {
	for _, cell := range guard.Cells {
		if !request.Budget.Spend() {
			return false
		}
		first, second := opposingOutcomes(cell)
		if first == ssaflow.OutcomeAny {
			continue
		}
		one := guard.Completes(request, ssaflow.FixedValues{cell: first})
		other := guard.Completes(request, ssaflow.FixedValues{cell: second})
		if one == ssaflow.EvidenceProven && other == ssaflow.EvidenceDisproven ||
			one == ssaflow.EvidenceDisproven && other == ssaflow.EvidenceProven {
			return true
		}
	}
	return false
}

// opposingOutcomes returns the two outcomes a named result can be fixed to:
// nil and non-nil, or true and false.
func opposingOutcomes(cell *ssa.Alloc) (ssaflow.Outcome, ssaflow.Outcome) {
	pointer, ok := cell.Type().Underlying().(*types.Pointer)
	if !ok {
		return ssaflow.OutcomeAny, ssaflow.OutcomeAny
	}
	if ssaflow.Nilable(pointer.Elem()) {
		return ssaflow.OutcomeNil, ssaflow.OutcomeNonNil
	}
	if basic, ok := pointer.Elem().Underlying().(*types.Basic); ok && basic.Info()&types.IsBoolean != 0 {
		return ssaflow.OutcomeTrue, ssaflow.OutcomeFalse
	}
	return ssaflow.OutcomeAny, ssaflow.OutcomeAny
}

// Completes asks whether the deferred literal completes the target on every
// one of its returns, given what its captured named results hold.
func (guard ResultGuard) Completes(request CompletionRequest, fixed ssaflow.FixedValues) ssaflow.EvidenceState {
	request.Instruction, request.Coverage, request.Constants = guard.Defer, CoverageEveryReturn, fixed
	return ProveCompletion(request).State
}

// CompletesAtReturn asks whether the deferred literal completes the target
// when the function leaves through returned, which it must dominate. Each
// named result is fixed to the outcome outcomeOf gives the value the return
// stores; a value with no known outcome, or a result the return does not set
// itself, leaves the answer unknown. Cell binding and callback invocation
// spend request.Budget; callers also use it for outcome inference. A callback
// may not publish an outcome after exhausting that allowance.
func (guard ResultGuard) CompletesAtReturn(
	request CompletionRequest, returned *ssa.Return, outcomeOf func(ssa.Value) (ssaflow.Outcome, bool),
) ssaflow.EvidenceState {
	fixed := ssaflow.FixedValues{}
	for _, cell := range guard.Cells {
		if !request.Budget.Spend() {
			return ssaflow.EvidenceUnknown
		}
		value, ok := ssaflow.ValueAtReturnWithin(returned, cell, request.Budget)
		if !ok {
			return ssaflow.EvidenceUnknown
		}
		if !request.Budget.Spend() {
			return ssaflow.EvidenceUnknown
		}
		outcome, ok := outcomeOf(value)
		if !ok || request.Budget.Exhausted() || request.Budget.PoolExhausted() {
			return ssaflow.EvidenceUnknown
		}
		fixed[cell] = outcome
	}
	return guard.Completes(request, fixed)
}

// ProveReachesReturn distinguishes a defer registered on every path to
// returned from one that may reach it or is disconnected. Interrupted order
// or reachability searches remain unknown, never evidence of disconnection.
func (guard ResultGuard) ProveReachesReturn(returned *ssa.Return, budget *ssaflow.SearchBudget) ssaflow.Proof {
	state := ssaflow.EvidenceDisproven
	if ssaflow.InstructionDominatesWithin(guard.Defer, returned, budget) {
		state = ssaflow.EvidenceProven
	} else if ssaflow.InstructionMayFollowWithin(guard.Defer, returned, budget) {
		state = ssaflow.EvidenceUnknown
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
	}
	return ssaflow.Proof{State: state, Reason: ssaflow.EvidenceStructuralWalk, Provenance: ssaflow.EvidenceFromLocalSSA}
}
