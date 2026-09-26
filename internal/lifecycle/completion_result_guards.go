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

// ResultGuards returns the deferred literals of function whose completion,
// asked by request on every return of the literal, is proven under one
// outcome of a captured named result and disproven under the other.
// Instruction, Coverage, and Constants of request are set per question.
func ResultGuards(function *ssa.Function, request CompletionRequest) []ResultGuard {
	var guards []ResultGuard
	for _, deferred := range ssaflow.InstructionsOf[*ssa.Defer](function) {
		closure, ok := deferred.Call.Value.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		var cells []*ssa.Alloc
		for _, binding := range closure.Bindings {
			if cell, ok := binding.(*ssa.Alloc); ok {
				if _, named := ssaflow.NamedResultCell(function, cell); named {
					cells = append(cells, cell)
				}
			}
		}
		guard := ResultGuard{Defer: deferred, Cells: cells}
		if len(cells) != 0 && guard.turnsOnResult(request) {
			guards = append(guards, guard)
		}
	}
	return guards
}

func (guard ResultGuard) turnsOnResult(request CompletionRequest) bool {
	for _, cell := range guard.Cells {
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
// itself, leaves the answer unknown.
func (guard ResultGuard) CompletesAtReturn(
	request CompletionRequest, returned *ssa.Return, outcomeOf func(ssa.Value) (ssaflow.Outcome, bool),
) ssaflow.EvidenceState {
	fixed := ssaflow.FixedValues{}
	for _, cell := range guard.Cells {
		value, ok := ssaflow.ValueAtReturn(returned, cell)
		if !ok {
			return ssaflow.EvidenceUnknown
		}
		outcome, ok := outcomeOf(value)
		if !ok {
			return ssaflow.EvidenceUnknown
		}
		fixed[cell] = outcome
	}
	return guard.Completes(request, fixed)
}
