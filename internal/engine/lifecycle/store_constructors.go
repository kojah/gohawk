package lifecycle

import (
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/syntax"
	"golang.org/x/tools/go/ssa"
)

// Constructor delegation preserves an every-successful-return owner guarantee
// while reusing the surrounding possible-containment search. Argument binding,
// body census and return coverage share its allowance; cutoff cannot establish
// a constructor guarantee or the nil/error-only exception.

func (search *ownershipSearch) callAggregateStoresValue(call *ssa.Call, value ssa.Value) bool {
	common := call.Common()
	if ssacall.CallMatchesSymbol(common, syntax.Builtin("append")) && search.anyAggregateStoresValue(common.Args, value) {
		return true
	}
	callee := ssacall.ResolvedCallee(common)
	if callee == nil {
		return false
	}
	for index, argument := range common.Args {
		if !search.budget.Spend() {
			return false
		}
		if !search.aggregateStoresValue(argument, value) {
			continue
		}
		if index < len(callee.Params) && search.functionReturnsOwner(callee, callee.Params[index]) {
			return true
		}
		// A callee in another package has neither a body nor parameter values
		// here, so its summary is the only account of what it did with the
		// argument. Index the summary by the argument position, which is how
		// the summary counts parameters, receiver included.
		if search.summarized != nil && search.budget.Spend() && search.summarized(callee, index) {
			return true
		}
	}
	return false
}

func (search *ownershipSearch) functionReturnsOwner(function *ssa.Function, value ssa.Value) bool {
	// Returned-owner summaries carry an every-return guarantee. Preserve it
	// while following a delegated constructor: one branch returning an owner
	// cannot justify transferring the parameter on a sibling branch that
	// returns an unrelated value. Nil/error-only returns carry no owner and are
	// the same unsuccessful-construction exception used by the outer proof.
	owners := map[*ssa.Return]bool{}
	hasOwner := false
	for instruction := range ssaflow.InstructionsWithin(function, search.budget) {
		returned, ok := instruction.(*ssa.Return)
		if !ok {
			continue
		}
		owners[returned] = search.returnedValueOwnsValue(returned, value)
		hasOwner = hasOwner || owners[returned]
	}
	if !hasOwner || search.exhausted() {
		return false
	}
	return ssapath.EvaluateObligationFromEntry(function, ssapath.ObligationFlow{
		Budget:      search.budget,
		Instruction: func(ssa.Instruction) ssapath.ObligationAction { return ssapath.ObligationNone },
		Return: func(returned *ssa.Return) ssapath.ObligationAction {
			if owners[returned] || ssaflow.ReturnsOnlyNilOrErrorsWithin(returned, search.budget) {
				return ssapath.ObligationExact
			}
			return ssapath.ObligationNone
		},
	}) == ssapath.ObligationHonored && !search.exhausted()
}

// callStoresValueIntoAggregate reports whether a call hands a callee both the
// aggregate and the value, and that callee stores the value into it.
//
// A constructor commonly delegates the assembly of the value it returns rather
// than writing the field itself: bufio.NewReader reaches its buffer through
// (*Reader).reset, and encoding/json's NewDecoder reaches its reader through
// jsontext.NewDecoder and then (*Decoder).Reset. The parameter is stored into
// the returned aggregate only inside those callees, so a search that stops at
// the caller's own stores concludes the callee kept the value for itself, and
// the caller is then wrongly credited with handing over the release. The
// helper is often unexported, so its summary is never exported and the answer
// has to come from its body, which is available here for a callee in the same
// package.
func (search *ownershipSearch) callStoresValueIntoAggregate(call ssa.CallInstruction, aggregate, value ssa.Value) bool {
	common := call.Common()
	if common == nil {
		return false
	}
	callee := ssacall.ResolvedCallee(common)
	if callee == nil || len(callee.Blocks) == 0 {
		return false
	}
	// The aggregate may be handed over as a field of itself, as jsontext does
	// when Reset passes d.s to the state's own reset.
	holder := -1
	for index, argument := range common.Args {
		if !search.budget.Spend() {
			return false
		}
		if index < len(callee.Params) &&
			(search.aliases(argument, aggregate) || ssaflow.ValueIsAccessPathFromWithin(argument, aggregate, search.budget)) {
			holder = index
			break
		}
	}
	if holder < 0 {
		return false
	}
	for index, argument := range common.Args {
		if !search.budget.Spend() {
			return false
		}
		if index == holder || index >= len(callee.Params) {
			continue
		}
		// A callback that captured the value is not the value being stored.
		// Registering one, as t.Cleanup does, keeps the callback alive without
		// proving the callback releases anything, and the closure rules decide
		// that separately. Reading it as a store would accept a cleanup that
		// only conditionally releases.
		if _, closure := argument.(*ssa.MakeClosure); closure {
			continue
		}
		if !search.aggregateStoresValue(argument, value) {
			continue
		}
		if search.aggregateStoresValue(callee.Params[holder], callee.Params[index]) {
			return true
		}
	}
	return false
}
