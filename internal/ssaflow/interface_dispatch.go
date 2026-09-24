package ssaflow

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// DispatchReason describes exact interface receiver resolution, independently
// of whether the resolved function has a body or usable effect summary.
type DispatchReason uint8

const (
	DispatchUnknown DispatchReason = iota
	DispatchConcreteReceiver
	DispatchBudgetExhausted
)

// InterfaceDispatch names a concrete method and its unboxed receiver. It
// provides dispatch identity only, not purity, termination, or lifecycle facts.
type InterfaceDispatch struct {
	Function *ssa.Function
	Receiver ssa.Value
	Reason   DispatchReason
}

// Proven reports exact agreement on the receiver and method.
func (dispatch InterfaceDispatch) Proven() bool { return dispatch.Reason == DispatchConcreteReceiver }

// ResolveInterfaceDispatch resolves direct interface boxes and agreeing phi
// alternatives through interface conversions. Loads, arbitrary interface
// parameters, and alternatives with different receivers remain unknown.
// The supplied program owns method-wrapper construction; no SSA is fabricated.
func ResolveInterfaceDispatch(common *ssa.CallCommon, program *ssa.Program, budget *SearchBudget) InterfaceDispatch {
	if common == nil || !common.IsInvoke() || program == nil {
		return InterfaceDispatch{Reason: DispatchUnknown}
	}
	receiver, known := ResolveReachingValue(
		NewReachingWalk(TransparentChangeInterface), common.Value,
		func(_ ReachingWalk, value ssa.Value) (ssa.Value, bool) {
			boxed, ok := value.(*ssa.MakeInterface)
			if !budget.Spend() || !ok {
				return nil, false
			}
			return boxed.X, true
		}, func(value ssa.Value) ssa.Value { return value },
	)
	if budget.Exhausted() {
		return InterfaceDispatch{Reason: DispatchBudgetExhausted}
	}
	if !known {
		return InterfaceDispatch{Reason: DispatchUnknown}
	}
	selection := types.NewMethodSet(receiver.Type()).Lookup(common.Method.Pkg(), common.Method.Name())
	if selection == nil {
		return InterfaceDispatch{Reason: DispatchUnknown}
	}
	function := program.MethodValue(selection)
	if function == nil {
		return InterfaceDispatch{Reason: DispatchUnknown}
	}
	return InterfaceDispatch{Function: function, Receiver: receiver, Reason: DispatchConcreteReceiver}
}
