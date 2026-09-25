package lifecyclefacts

import (
	"slices"
	"strconv"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"

	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Retention evidence comes in two strengths because its consumers pull in
// opposite directions. Retained may over-approximate: an opaque callee, an
// interface invoke, or any literal capture counts, so a consumer that widens
// "unknown" errs toward suppression. Stored must under-approximate: only a
// store into a global, a field, a map, a channel, an append, a return, an
// escaping literal, or a callee proven to store counts, so a consumer that
// treats it as an ownership transfer never transfers on a guess. Both follow
// same-package helpers with bodies rather than assuming, so a copy loop that
// only reads through its argument marks nothing. Values merely derived from
// the parameter, such as a loaded field, never count.

// retentionBudget bounds one retention question by the number of instructions
// it may examine. The walk descends the callee graph, and a cycle guard cuts
// an answer short without letting it be memoized, so a mutually recursive
// package can otherwise re-walk the same helpers indefinitely. Exhausting the
// budget returns what the cycle guard returns, and the two walks answer with
// opposite polarity so that neither invents evidence. The loose walk
// over-approximates what a callee may keep, so it bails to "maybe retained"
// and leaves a suppression available. The strict walk claims a proven store,
// so it bails to "not proven" rather than fabricate an ownership transfer. A
// callee graph too large to walk is declined rather than analyzed halfway.
const retentionBudget = 250_000

type retention struct {
	pass        *analysis.Pass
	strict      bool
	everyReturn bool
	lookup      func(ssa.Instruction) (Fact, bool)
	// budget bounds the instructions this question may examine. The shared
	// type owns the counting; the polarity a bailout takes stays here, because
	// only this walk knows which answer refuses to invent evidence.
	budget *ssaflow.SearchBudget
	// memo owns the cycle guard and the rule that an answer the guard or the
	// budget cut short is not retained. It is shared for the whole package:
	// summarizing every exported function asks about the same helpers again
	// and again, and the guard is empty between questions.
	memo *ssaflow.CallGraphMemo[retentionKey, bool]
}

// retentionKey identifies one retention question. The strict and loose walks
// answer differently, so the mode is part of the key and both share one memo.
type retentionKey struct {
	function    *ssa.Function
	parameter   ssa.Value
	strict      bool
	everyReturn bool
}

// retentionCache shares retention answers across one package. A single walk
// descends the callee graph, and summarizing every exported function asks
// about the same helpers again and again, so the answers must outlive one
// question.
type retentionCache struct {
	memo   *ssaflow.CallGraphMemo[retentionKey, bool]
	lookup func(ssa.Instruction) (Fact, bool)
}

func newRetentionCache() *retentionCache {
	return &retentionCache{
		memo: ssaflow.NewCallGraphMemo[retentionKey, bool](),
	}
}

func (cache *retentionCache) retainedAnywhere(pass *analysis.Pass, function *ssa.Function, parameter ssa.Value) bool {
	return (&retention{pass: pass, budget: ssaflow.NewSearchBudget(retentionBudget), memo: cache.memo, lookup: cache.lookup}).answer(function, parameter)
}

// Visible private helpers have no exported fact. Require a retention witness
// on every return before using their bodies as a caller ownership boundary.
func (cache *retentionCache) storedEveryReturn(pass *analysis.Pass, function *ssa.Function, parameter ssa.Value) bool {
	return (&retention{
		pass: pass, strict: true, everyReturn: true,
		budget: ssaflow.NewSearchBudget(retentionBudget), memo: cache.memo, lookup: cache.lookup,
	}).answer(function, parameter)
}

// answer runs one top-level retention question and traces a budget bailout,
// which is a conservative boundary rather than a proof.
func (search *retention) answer(function *ssa.Function, parameter ssa.Value) bool {
	retained := search.within(function, parameter)
	if !search.budget.Exhausted() {
		return retained
	}
	analysisTrace.For(search.pass, traceAnalyzer, "", function.Pos()).Considered(analysisTrace.Step{
		Reason:   reasonRetentionBudget.String(),
		Outcome:  analysisTrace.OutcomeUnknown,
		Pos:      function.Pos(),
		Function: function.String(),
		Details:  map[string]string{"strict": strconv.FormatBool(search.strict)},
	})
	return retained
}

func (search *retention) within(function *ssa.Function, parameter ssa.Value) bool {
	if function == nil || len(function.Blocks) == 0 {
		return false
	}
	key := retentionKey{function: function, parameter: parameter, strict: search.strict, everyReturn: search.everyReturn}
	retained := !search.strict
	// Guard before looking up the answer: a recursive call contributes only
	// may-retain evidence, even if another context already populated the cache.
	search.memo.WithFunction(function, func() {
		retained = search.memo.Compose(key, search.budget, func() bool {
			return search.searchWithin(function, parameter)
		}, func(_ ssaflow.SummaryUnavailable, partial bool) bool {
			// Preserve an independently established store; lack of one may only
			// support the loose may-retain fallback after budget exhaustion.
			return partial || !search.strict
		})
	})
	return retained
}

func (search *retention) searchWithin(function *ssa.Function, parameter ssa.Value) bool {
	// The parameter itself, or a whole copy of an aggregate that holds it:
	// storing or returning such a copy keeps the parameter just as surely.
	derives := func(value ssa.Value) bool {
		return heapmodel.MayAlias(value, parameter) || lifecycle.LoadedAggregateMayHold(value, parameter)
	}
	if search.everyReturn {
		return lifecycle.MethodCallCoverage(function, func(instruction ssa.Instruction) bool {
			return search.budget.Spend() && search.instructionRetains(function, instruction, derives)
		}, lifecycle.CoverageEveryReturn, parameter)
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !search.budget.Spend() {
				return !search.strict
			}
			if search.instructionRetains(function, instruction, derives) {
				return true
			}
		}
	}
	return false
}

func (search *retention) instructionRetains(function *ssa.Function, instruction ssa.Instruction, derives func(ssa.Value) bool) bool {
	switch typed := instruction.(type) {
	case *ssa.Store:
		return search.storeRetains(function, typed, derives)
	case *ssa.MakeClosure:
		// Capturing a resource for later work is not a completed ownership
		// handoff. Cleanup registrations need their separate completion proof;
		// a callback that only conditionally closes must not settle its input.
		if search.everyReturn {
			return false
		}
		if !slices.ContainsFunc(typed.Bindings, func(binding ssa.Value) bool {
			return derives(binding) || derives(ssaflow.CapturedBindingValue(binding))
		}) {
			return false
		}
		return !search.strict || search.closureEscapes(typed)
	case *ssa.Send:
		return !search.everyReturn && derives(typed.X)
	case *ssa.MapUpdate:
		return !search.everyReturn && derives(typed.Value)
	case *ssa.Return:
		// Returning the value hands it to the caller, which the returned-owner
		// and view summaries describe; strict retention leaves it to them.
		return !search.strict && slices.ContainsFunc(typed.Results, derives)
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return search.callRetains(ssaflow.InstructionCall(instruction), instruction, derives)
	}
	return false
}

// storeRetains decides the Store case of instructionRetains.
func (search *retention) storeRetains(function *ssa.Function, typed *ssa.Store, derives func(ssa.Value) bool) bool {
	// A store into a local allocation, or into a field or element of one,
	// keeps the value only as long as the local lives. In strict mode that
	// is not retention: a local aggregate that escapes is decided where it
	// escapes. The loose mode keeps counting it.
	wrapped := search.everyReturn && search.returnedWrapperContains(typed.Val, derives)
	if !derives(typed.Val) && !wrapped {
		return false
	}
	if search.everyReturn {
		// A direct global store has an owner independent of caller-local
		// storage. Parameter fields and local spills need a separate escape
		// proof at the caller, not just retention somewhere in this helper.
		_, global := typed.Addr.(*ssa.Global)
		return global
	}
	local, ok := localStorage(typed.Addr, function)
	// The builder's spill of a by-value parameter into a cell that is
	// only ever written whole and read cannot keep the value past the
	// call in either mode; it is the parameter's own copy, not storage.
	if ok && local == typed.Addr && ssaflow.WholeWrittenCell(local) {
		return false
	}
	if !ok || !search.strict {
		return true
	}
	return search.valueEscapes(local, map[ssa.Value]bool{})
}

// localStorage returns the local allocation of the function that the
// address is, or lies beneath through fields and elements.
func localStorage(address ssa.Value, function *ssa.Function) (*ssa.Alloc, bool) {
	for {
		switch typed := address.(type) {
		case *ssa.Alloc:
			return typed, typed.Parent() == function
		case *ssa.FieldAddr:
			address = typed.X
		case *ssa.IndexAddr:
			address = typed.X
		default:
			return nil, false
		}
	}
}

// closureEscapes reports whether a literal outlives its creation: it is
// launched, stored, sent, appended, or handed to a callee that keeps it, or
// returned in the loose mode. A literal only deferred or called in place does
// not escape, and a conversion to a named function type is followed to the
// converted value's own uses.
func (search *retention) closureEscapes(closure *ssa.MakeClosure) bool {
	return search.valueEscapes(closure, map[ssa.Value]bool{})
}

func (search *retention) valueEscapes(value ssa.Value, seen map[ssa.Value]bool) bool {
	if seen[value] || value.Referrers() == nil {
		return false
	}
	seen[value] = true
	for _, reference := range *value.Referrers() {
		if search.referenceEscapes(value, reference, seen) {
			return true
		}
	}
	return false
}

func (search *retention) referenceEscapes(value ssa.Value, reference ssa.Instruction, seen map[ssa.Value]bool) bool {
	isValue := func(candidate ssa.Value) bool { return candidate == value }
	switch typed := reference.(type) {
	case *ssa.Defer:
		return typed.Common().Value != value && search.callRetains(typed.Common(), typed, isValue)
	case *ssa.Call:
		return typed.Common().Value != value && search.callRetains(typed.Common(), typed, isValue)
	case *ssa.Return:
		return !search.strict
	case *ssa.ChangeType, *ssa.Convert, *ssa.MakeInterface, *ssa.ChangeInterface, *ssa.FieldAddr, *ssa.IndexAddr, *ssa.Slice:
		// Conversions and projections of an aggregate escape when they do.
		converted, _ := typed.(ssa.Value)
		return search.valueEscapes(converted, seen)
	case *ssa.Store:
		// Storing into the value is not an escape of the value. Storing the
		// value into a local cell escapes only if that cell does; storing it
		// anywhere else is an escape.
		if typed.Addr == value {
			return false
		}
		if local, ok := localStorage(typed.Addr, typed.Parent()); ok {
			return search.valueEscapes(local, seen)
		}
		return true
	case *ssa.MakeClosure:
		// A cell captured by a literal escapes only if the literal does.
		return search.valueEscapes(typed, seen)
	case *ssa.UnOp, *ssa.DebugRef:
		return false
	default:
		return true
	}
}

// callRetains treats a summarized callee as retaining what its summary marks
// and a same-package callee with a body as retaining what its body retains.
// An opaque callee or an interface invoke retains in the loose mode and does
// not in the strict one.
func (search *retention) callRetains(common *ssa.CallCommon, instruction ssa.Instruction, derives func(ssa.Value) bool) bool {
	if common == nil {
		return false
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		return !search.everyReturn && builtin.Name() == "append" && anyArgument(common, derives)
	}
	if imported, ok := search.fact(instruction); ok {
		mask := imported.Claim(ClaimRetains)
		if search.strict {
			mask = imported.Claim(ClaimStores)
			if search.everyReturn {
				// Construction alone does not retain a wrapper beyond this
				// helper. Its actual escaping store must cover each return.
				mask &^= imported.ReturnedOwner()
			}
		}
		return argumentInMask(common, mask, derives)
	}
	callee := common.StaticCallee()
	if callee == nil || len(callee.Blocks) == 0 {
		return !search.strict && anyArgument(common, derives)
	}
	for _, binding := range ssaflow.CallBindings(common, callee, nil) {
		if !derives(binding.Supplied) {
			continue
		}
		if search.within(callee, binding.Local) {
			return true
		}
	}
	return false
}

// Returned-owner and strict-storage claims supply positive parameter-to-wrapper
// provenance. Merely calling a may-retain API supplies no such relationship.
// The store is classified at its real position, so an earlier error return
// cannot borrow evidence from a later logger installation.
// https://github.com/shijuvar/gokit/blob/4b5abbb8d4e6497a1eef211cb823c18b7977dde4/log/log.go#L34-L51
func (search *retention) returnedWrapperContains(value ssa.Value, derives func(ssa.Value) bool) bool {
	call, ok := value.(*ssa.Call)
	if !ok || call.Common().Signature().Results().Len() != 1 {
		return false
	}
	fact, known := search.fact(call)
	return known && argumentInMask(call.Common(), fact.ReturnedOwner()&fact.Stored(), derives)
}

// During prerequisite construction facts are imported directly; a consumer's
// pass instead owns the prerequisite result, not its object-fact namespace.
func (search *retention) fact(instruction ssa.Instruction) (Fact, bool) {
	if search.lookup != nil {
		return search.lookup(instruction)
	}
	return importFact(search.pass, instruction)
}

func anyArgument(common *ssa.CallCommon, derives func(ssa.Value) bool) bool {
	return slices.ContainsFunc(common.Args, derives)
}

func argumentInMask(common *ssa.CallCommon, mask ParameterMask, derives func(ssa.Value) bool) bool {
	for index, argument := range common.Args {
		if mask.contains(index) && derives(argument) {
			return true
		}
	}
	return false
}
