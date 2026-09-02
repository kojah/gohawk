package lifecyclefacts

import (
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"

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

type retention struct {
	pass   *analysis.Pass
	strict bool
	seen   map[*ssa.Function]bool
}

func retainedAnywhere(pass *analysis.Pass, function *ssa.Function, parameter ssa.Value) bool {
	return (&retention{pass: pass, seen: map[*ssa.Function]bool{}}).within(function, parameter)
}

func storedAnywhere(pass *analysis.Pass, function *ssa.Function, parameter ssa.Value) bool {
	return (&retention{pass: pass, strict: true, seen: map[*ssa.Function]bool{}}).within(function, parameter)
}

func (search *retention) within(function *ssa.Function, parameter ssa.Value) bool {
	if search.seen[function] {
		return false
	}
	search.seen[function] = true
	defer delete(search.seen, function)
	derives := func(value ssa.Value) bool {
		return ssaflow.SameValue(value, parameter)
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
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
		// A store into a local allocation, or into a field or element of one,
		// keeps the value only as long as the local lives. In strict mode that
		// is not retention: a local aggregate that escapes is decided where it
		// escapes. The loose mode keeps counting it.
		if !derives(typed.Val) {
			return false
		}
		local, ok := localStorage(typed.Addr, function)
		if !ok || !search.strict {
			return true
		}
		return search.valueEscapes(local, map[ssa.Value]bool{})
	case *ssa.MakeClosure:
		if !slices.ContainsFunc(typed.Bindings, func(binding ssa.Value) bool {
			return derives(binding) || derives(ssaflow.CapturedBindingValue(binding))
		}) {
			return false
		}
		return !search.strict || search.closureEscapes(typed)
	case *ssa.Send:
		return derives(typed.X)
	case *ssa.MapUpdate:
		return derives(typed.Value)
	case *ssa.Return:
		// Returning the value hands it to the caller, which the returned-owner
		// and view summaries describe; strict retention leaves it to them.
		return !search.strict && slices.ContainsFunc(typed.Results, derives)
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return search.callRetains(ssaflow.InstructionCall(instruction), instruction, derives)
	}
	return false
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
		return builtin.Name() == "append" && anyArgument(common, derives)
	}
	if imported, ok := importFact(search.pass, instruction); ok {
		mask := imported.Retained
		if search.strict {
			mask = imported.Stored
		}
		return argumentInMask(common, mask, derives)
	}
	callee := common.StaticCallee()
	if callee == nil || len(callee.Blocks) == 0 {
		return !search.strict && anyArgument(common, derives)
	}
	for index, argument := range common.Args {
		if index >= len(callee.Params) || !derives(argument) {
			continue
		}
		if search.seen[callee] {
			return !search.strict
		}
		if search.within(callee, callee.Params[index]) {
			return true
		}
	}
	return false
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
