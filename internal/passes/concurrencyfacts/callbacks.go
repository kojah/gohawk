package concurrencyfacts

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// A call through a function-typed parameter or capture runs code the caller
// chooses, and so does a method call on an interface-typed one. Rather than
// make the whole summary unknown, the summary keeps an Invoke operation at that
// position: a hole, not an effect. Binding a call site fills the hole with the
// supplied function's own bound summary, or, for an interface, with the method
// of the one concrete value the caller boxes, bound to that value. Otherwise
// the hole is forwarded to the caller's own input, or stays in the sequence,
// and the summary stays incomplete until it is filled. This follows the
// CancellationInputs model: requirements survive composition and fact
// publication by parameter position, and are discharged only by exact
// evidence at a call site. Contexts are not holes; the cancellation model
// owns them.
//
// The first slice is deliberately narrow. Every argument of the invocation
// must be inert, so the callback cannot reach a resource through its
// parameters; a supplied callback or method whose effects touch its own
// parameters, has branching paths, or lives in another package leaves the hole
// unknown. An interface is filled only from a visible boxing of one concrete
// value: an interface read from memory or merged from several values could
// hold any type. Holes inside launched workers, select arms, and deferred
// calls are not filled. A callback that launches a goroutine usually stays
// unknown too: its captured cells reach an asynchronous participant, which
// heapmodel does not prove stable across the enclosing call.

// callbackHole records a call through a function-typed or interface-typed
// parameter or capture.
func callbackHole(instruction ssa.CallInstruction) (Summary, bool) {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return Summary{}, false
	}
	common := call.Common()
	switch {
	case common.IsInvoke() && holeInput(common.Value, common.Method):
	case !common.IsInvoke() && common.StaticCallee() == nil && holeInput(common.Value, nil):
	default:
		return Summary{}, false
	}
	for _, argument := range common.Args {
		if !inertValue(argument.Type()) {
			return Summary{}, false
		}
	}
	hole := Operation{Kind: Invoke, Resource: Reference{Value: common.Value}, Method: common.Method, Source: call.Pos(), Site: call.Pos()}
	return Summary{Operations: []Operation{hole}}, true
}

// holeInput accepts only values a caller supplies directly: a parameter or a
// capture holding a function value, or, for a method hole, an interface value
// other than a context.
func holeInput(value ssa.Value, method *types.Func) bool {
	switch value.(type) {
	case *ssa.Parameter, *ssa.FreeVar:
	default:
		return false
	}
	if method != nil {
		return types.IsInterface(value.Type()) && !cancellationType(value.Type())
	}
	_, function := value.Type().Underlying().(*types.Signature)
	return function
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
// caller's value for the hole's input.
func (engine *Engine) bindCallback(result *Summary, hole Operation, supplied ssa.Value, instruction ssa.CallInstruction) Reason {
	if hole.Method != nil {
		// Converting between interfaces keeps the dynamic value, and so the
		// method a call dispatches to.
		for {
			inner, ok := ssaflow.UnwrapTransparentValue(supplied, ssaflow.TransparentChangeInterface)
			if !ok {
				break
			}
			supplied = inner
		}
	}
	if holeInput(supplied, hole.Method) {
		hole.Resource, hole.Site = Reference{Value: supplied}, instruction.Pos()
		result.Operations = append(result.Operations, hole)
		return ReasonNone
	}
	filled, reason := engine.suppliedCallback(supplied, hole.Method, instruction)
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

// suppliedCallback summarizes what the caller supplies for a hole and binds it
// to the caller's values: a function or closure with its captures, or the
// method of a boxed concrete value with that value as its receiver. The
// hole's own arguments have no binding here, so any effect on them makes the
// result unknown.
func (engine *Engine) suppliedCallback(supplied ssa.Value, method *types.Func, instruction ssa.CallInstruction) (Summary, Reason) {
	var function *ssa.Function
	var bindings []ssaflow.CallBinding
	switch value := supplied.(type) {
	case *ssa.Function:
		function = value
	case *ssa.MakeClosure:
		function, _ = value.Fn.(*ssa.Function)
		bindings = ssaflow.CallBindings(nil, function, value)
	case *ssa.MakeInterface:
		function = concreteMethod(instruction.Parent().Prog, value.X.Type(), method)
		if function != nil && len(function.Params) != 0 {
			bindings = []ssaflow.CallBinding{{Local: function.Params[0], Supplied: value.X}}
		}
	}
	if function == nil || len(function.Blocks) == 0 {
		return Summary{}, ReasonCallbackUnknown
	}
	summary := engine.summaries.Function(function, engine.budget)
	if !composableLinear(summary) {
		return Summary{}, ReasonCallbackUnknown
	}
	return engine.bindSummary(summary, bindings, instruction), ReasonNone
}

// concreteMethod returns the method a call of method dispatches to on a value
// of concrete type, as the language selects it.
func concreteMethod(program *ssa.Program, concrete types.Type, method *types.Func) *ssa.Function {
	if method == nil || types.IsInterface(concrete) {
		return nil
	}
	selection := program.MethodSets.MethodSet(concrete).Lookup(method.Pkg(), method.Name())
	if selection == nil {
		return nil
	}
	return program.MethodValue(selection)
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
