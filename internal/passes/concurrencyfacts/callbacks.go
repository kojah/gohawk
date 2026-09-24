package concurrencyfacts

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// A call through a function-typed parameter or capture runs code the caller
// chooses. Rather than make the whole summary unknown, the summary keeps an
// Invoke operation at that position: a hole, not an effect. Binding a call
// site fills the hole with the supplied function's own bound summary, or
// forwards it to the caller's own parameter. A hole that cannot be filled
// stays in the sequence, and the summary stays incomplete until it is. This
// follows the CancellationInputs model: requirements survive composition and
// fact publication by parameter position, and are discharged only by exact
// evidence at a call site.
//
// The first slice is deliberately narrow. Every argument of the invocation
// must be inert, so the callback cannot reach a resource through its
// parameters; a supplied callback whose effects touch its own parameters, has
// branching paths, or lives in another package leaves the hole unknown. Holes
// inside launched workers, select arms, and deferred calls are not filled. A
// callback that launches a goroutine usually stays unknown too: its captured
// cells reach an asynchronous participant, which heapmodel does not prove
// stable across the enclosing call.

// callbackHole records a call through a function-typed parameter or capture.
func callbackHole(instruction ssa.CallInstruction) (Summary, bool) {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return Summary{}, false
	}
	common := call.Common()
	if common.IsInvoke() || common.StaticCallee() != nil || !callbackInput(common.Value) {
		return Summary{}, false
	}
	for _, argument := range common.Args {
		if !inertValue(argument.Type()) {
			return Summary{}, false
		}
	}
	hole := Operation{Kind: Invoke, Resource: Reference{Value: common.Value}, Source: call.Pos(), Site: call.Pos()}
	return Summary{Operations: []Operation{hole}}, true
}

// callbackInput accepts only values a caller supplies directly: a parameter
// or a capture holding a function value.
func callbackInput(value ssa.Value) bool {
	switch value.(type) {
	case *ssa.Parameter, *ssa.FreeVar:
		_, function := value.Type().Underlying().(*types.Signature)
		return function
	default:
		return false
	}
}

// CallbacksBound reports whether every callback hole has been filled. Like
// CancellationBound, it is checked separately from Reason because a hole may
// sit inside an otherwise complete sequence.
func (summary Summary) CallbacksBound() bool {
	return !slices.ContainsFunc(summary.Operations, func(op Operation) bool { return op.Kind == Invoke })
}

// finishCallbacks marks a summary that still has holes, and clears the mark
// once binding has filled them all.
func finishCallbacks(summary Summary) Summary {
	switch {
	case summary.Reason == ReasonNone && !summary.CallbacksBound():
		summary.Reason = ReasonCallbackBindingRequired
	case summary.Reason == ReasonCallbackBindingRequired && summary.CallbacksBound():
		summary.Reason = ReasonNone
	}
	return summary
}

// bindCallback fills or forwards one hole at a call site. supplied is the
// caller's value for the hole's function input.
func (engine *Engine) bindCallback(result *Summary, hole Operation, supplied ssa.Value, instruction ssa.CallInstruction) Reason {
	if callbackInput(supplied) {
		hole.Resource, hole.Site = Reference{Value: supplied}, instruction.Pos()
		result.Operations = append(result.Operations, hole)
		return ReasonNone
	}
	filled, reason := engine.suppliedCallback(supplied, instruction)
	if reason != ReasonNone {
		return reason
	}
	if !composableLinear(filled) {
		// Keep the specific reason, such as an unbindable capture, for traces.
		return filled.Reason
	}
	if len(filled.Paths) != 0 || len(filled.Choices) != 0 || len(filled.deferred) != 0 {
		return ReasonCallbackUnknown
	}
	return appendCalled(result, filled, instruction)
}

// suppliedCallback summarizes a function or closure the caller passes and
// binds its captures to the caller's values. Its own parameters have no
// binding here, so any effect on them makes the result unknown.
func (engine *Engine) suppliedCallback(supplied ssa.Value, instruction ssa.CallInstruction) (Summary, Reason) {
	var function *ssa.Function
	var closure *ssa.MakeClosure
	switch value := supplied.(type) {
	case *ssa.Function:
		function = value
	case *ssa.MakeClosure:
		closure = value
		function, _ = value.Fn.(*ssa.Function)
	}
	if function == nil || len(function.Blocks) == 0 {
		return Summary{}, ReasonCallbackUnknown
	}
	summary := engine.summaries.Function(function, engine.budget)
	if !composableLinear(summary) {
		return Summary{}, ReasonCallbackUnknown
	}
	return engine.bindSummary(summary, ssaflow.CallBindings(nil, function, closure), instruction), ReasonNone
}

// holeSupplied returns the caller's value for a hole's function input.
func holeSupplied(hole Operation, bindings []ssaflow.CallBinding) (ssa.Value, bool) {
	for _, binding := range bindings {
		if binding.Local == hole.Resource.Value {
			return binding.Supplied, true
		}
	}
	return nil, false
}

// holeInstruction returns this function's call that created the first
// unfilled hole, so a cutoff explains which callback is missing. A hole
// forwarded from a callee has no call here and yields nil.
func holeInstruction(function *ssa.Function, summary Summary) ssa.Instruction {
	index := slices.IndexFunc(summary.Operations, func(op Operation) bool { return op.Kind == Invoke })
	if index < 0 || function == nil {
		return nil
	}
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
		if call.Pos() == summary.Operations[index].Source {
			return call
		}
	}
	return nil
}
