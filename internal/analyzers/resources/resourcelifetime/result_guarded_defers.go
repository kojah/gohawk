package resourcelifetime

import (
	"go/types"

	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/passes/resultfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Deferred releases guarded by a named result. A deferred literal runs after
// the return statement has set the function's named results, so a literal
// that closes only while err is non-nil, the close-on-error idiom, releases
// on some returns and not on others. Crediting it at the defer, as the
// data-dependent policy does for other guards, hides the success path that
// keeps the resource and hands it to nobody. Instead the defer settles
// nothing where it runs, and each return it dominates asks the shared
// completion search whether the literal releases given the value that return
// stores: a nil literal skips the cleanup, a value never nil runs it, and a
// value of unknown nilness leaves the resource unknown on that path.
//
// A literal counts as result-guarded only when the answer really turns on
// the result: it releases under one outcome of a captured named result and
// not under the other. A guard on any other variable, such as the committed
// flag of the transaction idiom, keeps the data-dependent policy.
// https://github.com/grpc/grpc-go/commit/db35da8bc5e8dcfcb57b94e9be0fba306710cc77

type resultGuardedDefer struct {
	deferred *ssa.Defer
	// cells are the named-result cells the literal captures.
	cells []*ssa.Alloc
}

func (analysis *resourceAnalysis) findResultGuardedDefers() []resultGuardedDefer {
	var guarded []resultGuardedDefer
	for _, deferred := range ssaflow.InstructionsOf[*ssa.Defer](analysis.function) {
		closure, ok := deferred.Call.Value.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		var cells []*ssa.Alloc
		for _, binding := range closure.Bindings {
			if cell, ok := binding.(*ssa.Alloc); ok {
				if _, named := ssaflow.NamedResultCell(analysis.function, cell); named {
					cells = append(cells, cell)
				}
			}
		}
		if len(cells) != 0 && analysis.releaseTurnsOnResult(deferred, cells) {
			guarded = append(guarded, resultGuardedDefer{deferred: deferred, cells: cells})
		}
	}
	return guarded
}

// releaseTurnsOnResult reports whether fixing one named result to one
// outcome makes the literal release on every return and fixing it to the
// opposite outcome does not.
func (analysis *resourceAnalysis) releaseTurnsOnResult(deferred *ssa.Defer, cells []*ssa.Alloc) bool {
	for _, cell := range cells {
		first, second := opposingOutcomes(cell)
		if first == ssaflow.OutcomeAny {
			continue
		}
		one := analysis.deferredRelease(deferred, ssaflow.FixedValues{cell: first})
		other := analysis.deferredRelease(deferred, ssaflow.FixedValues{cell: second})
		if one == ssaflow.EvidenceProven && other == ssaflow.EvidenceDisproven ||
			one == ssaflow.EvidenceDisproven && other == ssaflow.EvidenceProven {
			return true
		}
	}
	return false
}

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

// deferredRelease asks whether the deferred literal releases the resource on
// every one of its returns, given what its captured named results hold.
func (analysis *resourceAnalysis) deferredRelease(deferred *ssa.Defer, fixed ssaflow.FixedValues) ssaflow.EvidenceState {
	state := ssaflow.EvidenceDisproven
	for _, method := range analysis.contract.cleanup {
		completion := lifecycle.CompletionRequest{
			Instruction: deferred, Target: analysis.resource, Methods: []string{method},
			Coverage: lifecycle.CoverageEveryReturn, Constants: fixed, Budget: analysis.budget(releaseSearchBudget),
		}
		proof := analysis.evidence.Prove(lifecyclefacts.EvidenceRequest{Instruction: deferred, Target: analysis.resource, Completion: &completion})
		switch {
		case proof.Proven():
			return ssaflow.EvidenceProven
		case !proof.Known():
			state = ssaflow.EvidenceUnknown
		}
	}
	return state
}

// resultGuardedLabel labels a result-guarded defer, which settles nothing
// where it runs, and a return it may reach.
func (analysis *resourceAnalysis) resultGuardedLabel(instruction ssa.Instruction) (resourceAction, resourceLifetimeReason, bool) {
	switch typed := instruction.(type) {
	case *ssa.Defer:
		if analysis.resultGuarded(typed) {
			return actionNone, resourceReasonResultGuardedDefer, true
		}
	case *ssa.Return:
		return analysis.resultGuardedReturn(typed)
	}
	return actionNone, resourceReasonNone, false
}

func (analysis *resourceAnalysis) resultGuarded(deferred *ssa.Defer) bool {
	for _, guarded := range analysis.guardedDefers {
		if guarded.deferred == deferred {
			return true
		}
	}
	return false
}

// resultGuardedReturn labels a return by the result-guarded defers that
// reach it: settled when one releases given the values this return stores,
// unknown when a defer may or may not be registered on the way, or when a
// stored value's outcome is not known. It declines when every such defer
// provably skips the cleanup, so the return keeps its ordinary label.
func (analysis *resourceAnalysis) resultGuardedReturn(returned *ssa.Return) (resourceAction, resourceLifetimeReason, bool) {
	uncertain := false
	for _, guarded := range analysis.guardedDefers {
		if !ssaflow.InstructionDominates(guarded.deferred, returned) {
			if ssaflow.InstructionMayFollow(guarded.deferred, returned) {
				uncertain = true
			}
			continue
		}
		fixed, known := analysis.valuesAtReturn(returned, guarded.cells)
		if !known {
			uncertain = true
			continue
		}
		switch analysis.deferredRelease(guarded.deferred, fixed) {
		case ssaflow.EvidenceProven:
			return actionSettled, resourceReasonResultGuardedRelease, true
		case ssaflow.EvidenceUnknown:
			uncertain = true
		case ssaflow.EvidenceDisproven:
		}
	}
	if uncertain {
		return actionUnknown, resourceReasonResultGuardedUnknown, true
	}
	return actionNone, resourceReasonNone, false
}

// valuesAtReturn fixes each named result to what the return stores into it:
// a literal, a value never nil, or a call proven always nil or never nil.
func (analysis *resourceAnalysis) valuesAtReturn(returned *ssa.Return, cells []*ssa.Alloc) (ssaflow.FixedValues, bool) {
	fixed := ssaflow.FixedValues{}
	for _, cell := range cells {
		value, ok := ssaflow.ValueAtReturn(returned, cell)
		if !ok {
			return nil, false
		}
		outcome, ok := ssaflow.ValueOutcome(value)
		if !ok {
			outcome, ok = guaranteedOutcome(analysis.summaries.ResultOf(value, analysis.budget(releaseSearchBudget)))
		}
		if !ok {
			return nil, false
		}
		fixed[cell] = outcome
	}
	return fixed, true
}

// guaranteedOutcome turns a callee's result guarantee into the outcome it
// fixes.
func guaranteedOutcome(guarantee resultfacts.Guarantee) (ssaflow.Outcome, bool) {
	switch guarantee {
	case resultfacts.AlwaysNil:
		return ssaflow.OutcomeNil, true
	case resultfacts.AlwaysNonNil:
		return ssaflow.OutcomeNonNil, true
	case resultfacts.AlwaysTrue:
		return ssaflow.OutcomeTrue, true
	case resultfacts.AlwaysFalse:
		return ssaflow.OutcomeFalse, true
	case resultfacts.Unknown:
	}
	return ssaflow.OutcomeAny, false
}
